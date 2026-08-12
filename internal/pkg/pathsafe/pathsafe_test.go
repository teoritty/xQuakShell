package pathsafe_test

import (
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/pkg/pathsafe"
)

func TestUnderRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins", "com.example.p")

	cases := []struct {
		target string
		want   bool
	}{
		{root, true},
		{filepath.Join(root, "data", "file.txt"), true},
		{filepath.Join(root, "..", "other"), false},
	}
	for _, tc := range cases {
		if got := pathsafe.UnderRoot(root, tc.target); got != tc.want {
			t.Fatalf("UnderRoot(%q, %q) = %v want %v", root, tc.target, got, tc.want)
		}
	}
}

// TestPathsThatAreNotUTF8AreRefusedRatherThanReasonedAbout pins the guard a fuzz run bought.
//
// FuzzParseShimArgs produced a data root of two backslashes, the raw byte 0xE0 and a share name.
// Windows accepts that as a UNC volume, filepath.Abs round-trips it through the OS and hands back a
// replacement character in place of the byte, and filepath.Rel then spins forever comparing the
// mangled volume against itself - inside the function that decides whether an SFTP path escaped its
// root. The answer is to refuse the input, not to make the loop faster: a path whose bytes the OS
// will not preserve cannot be checked on the caller's behalf, whatever the check concludes.
func TestPathsThatAreNotUTF8AreRefusedRatherThanReasonedAbout(t *testing.T) {
	root := `\\` + string([]byte{0xE0}) + `\0`

	if pathsafe.UnderRoot(root, root+string(filepath.Separator)) {
		t.Error("UnderRoot said yes for a root that is not valid UTF-8; the deny direction is the " +
			"only safe answer when the bytes will not survive being resolved")
	}
	if _, err := pathsafe.ResolveUnderRoot(root, ""); err == nil {
		t.Error("ResolveUnderRoot accepted a root that is not valid UTF-8")
	}
	if _, err := pathsafe.ResolveUnderRoot(t.TempDir(), string([]byte{0xE0})); err == nil {
		t.Error("ResolveUnderRoot accepted a target that is not valid UTF-8")
	}
	// A path that merely contains non-ASCII is ordinary and must still work, or this guard has
	// quietly become a ban on everyone whose files are not named in English.
	root = filepath.Join(t.TempDir(), "плагины")
	if !pathsafe.UnderRoot(root, filepath.Join(root, "данные")) {
		t.Error("UnderRoot refused a valid non-ASCII path; the guard is on invalid UTF-8, not on " +
			"characters outside ASCII")
	}
}

// TestARootSpeltDifferentlyFromItsResolvedFormStillMatches covers the bug that made the plugin
// filesystem boundary refuse legitimate access.
//
// SecurePathUnderRoots resolves the final path with EvalSymlinks and then asked whether the RAW
// root prefixed it. Those are two spellings of the same directory whenever the root is reached
// through a link - or, on Windows, whenever it carries an 8.3 short name, which every user with a
// long account name has in their temp path. The CI runner is one of them: it refused
// C:\Users\RUNNER~1\... against a target that resolved to C:\Users\runneradmin\..., and thirty-odd
// tests failed with "path access denied" and "plugin capability denied".
//
// The link here stands in for the short name: both make EvalSymlinks return a different string for
// the same directory, which is the whole of the defect.
func TestARootSpeltDifferentlyFromItsResolvedFormStillMatches(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}
	wanted := filepath.Join(real, "data.txt")
	if err := os.WriteFile(wanted, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "root")
	if err := os.Symlink(real, root); err != nil {
		t.Skipf("this machine would not let the test create a symlink (%v), so the mismatched-"+
			"spelling boundary stays unverified here; the Windows form of it is in "+
			"pathsafe_shortpath_windows_test.go", err)
	}

	got, err := pathsafe.SecurePathUnderRoots(filepath.Join(root, "data.txt"), []string{root})
	if err != nil {
		t.Fatalf("SecurePathUnderRoots refused a file inside its own root: %v; a root and a "+
			"resolved path that name the same directory differently are still the same directory", err)
	}
	// Both sides in canonical form: t.TempDir() can itself hand back a spelling that differs from
	// the resolved one, which is the very thing under test.
	resolvedWant, err := filepath.EvalSymlinks(wanted)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(resolvedWant) {
		t.Errorf("resolved to %q, want %q", got, resolvedWant)
	}
}

func TestResolveUnderRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	_, err := pathsafe.ResolveUnderRoot(root, "../outside")
	if err == nil {
		t.Fatal("expected escape rejection")
	}
}
