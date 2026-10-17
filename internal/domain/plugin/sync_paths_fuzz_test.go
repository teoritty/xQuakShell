package plugin

import (
	"strings"
	"testing"
)

// safeAccepted is what every path the policy accepts must be, whatever produced it: relative,
// slash-separated, free of "." and ".." segments, colons, backslashes and control characters, and
// never under tmp/.
func safeAccepted(t *testing.T, rel string) {
	t.Helper()
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, `:\`) {
		t.Fatalf("accepted %q: not a relative slash path", rel)
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.EqualFold(seg, "tmp") {
			t.Fatalf("accepted %q: segment %q", rel, seg)
		}
		if strings.IndexFunc(seg, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			t.Fatalf("accepted %q: control character", rel)
		}
	}
}

// FuzzValidSyncFile feeds the file-path check what a hostile device could put in a plugin-data
// record (spec 12.3): it must never panic, and anything it accepts must be a safe relative path.
func FuzzValidSyncFile(f *testing.F) {
	for _, s := range []string{"state.json", "notes/a.md", "../vault.age", "/etc/passwd", `C:\x`, "a:b",
		"tmp/x", "a/./b", "nul.json", "a\x00b", "notes/", "a//b", "é/ü.txt", strings.Repeat("a/", 200)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, rel string) {
		if ValidSyncFile(rel) == nil {
			safeAccepted(t, rel)
		}
	})
}

// FuzzSyncSpecMatches feeds a declared pattern and a file: validation and matching never panic, and
// a file the policy refuses never matches, whatever the pattern.
func FuzzSyncSpecMatches(f *testing.F) {
	for _, s := range [][2]string{{"**", "a/b"}, {"state/**", "state/x"}, {"*.json", "a.json"}, {"**", "tmp/x"},
		{"**", "../x"}, {"a/*/c", "a/b/c"}, {"***", "x"}, {"", ""}} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pattern, rel string) {
		spec := &SyncSpec{Paths: []string{pattern}}
		_ = spec.Validate(IsolationPerPlugin)
		if spec.Matches(rel) {
			if err := ValidSyncFile(rel); err != nil {
				t.Fatalf("%q matched %q, which the policy refuses: %v", pattern, rel, err)
			}
			safeAccepted(t, rel)
		}
	})
}
