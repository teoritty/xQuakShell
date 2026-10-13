package plugin

import (
	"errors"
	"strings"
	"testing"
)

func TestSyncSpecValidateRefusesEveryUnsafePattern(t *testing.T) {
	refused := map[string]string{
		"empty":                "",
		"absolute":             "/etc/passwd",
		"backslash":            `state\db.json`,
		"drive letter":         "C:/Users/x",
		"alternate data":       "state.json:hidden",
		"traversal at front":   "../vault.age",
		"traversal in middle":  "a/../../vault.age",
		"dot segment":          "./state.json",
		"empty segment":        "a//b",
		"trailing slash":       "state/",
		"question glob":        "state?.json",
		"class glob":           "state[0-9].json",
		"brace glob":           "{a,b}.json",
		"double star in name":  "a**.json",
		"temp dir":             "tmp/cache.json",
		"temp dir nested":      "cache/TMP/x",
		"placeholder":          "${pluginData}/state.json",
		"home":                 "~/state.json",
		"device name":          "nul",
		"device with ext":      "cache/COM1.txt",
		"trailing dot":         "state.",
		"trailing space":       "state ",
		"control character":    "state\x01.json",
		"overlong":             strings.Repeat("a", 257),
	}
	for name, p := range refused {
		t.Run(name, func(t *testing.T) {
			s := &SyncSpec{Paths: []string{p}}
			if err := s.Validate(IsolationPerPlugin); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("sync path %q: %v, want ErrInvalidManifest", p, err)
			}
		})
	}
}

func TestSyncSpecValidateAcceptsDeclaredData(t *testing.T) {
	s := &SyncSpec{Paths: []string{"state.json", "profiles/*.json", "notes/**", "**/settings.toml"}}
	if err := s.Validate(IsolationPerPlugin); err != nil {
		t.Fatalf("well-formed sync paths refused: %v", err)
	}
	var none *SyncSpec
	if err := none.Validate(IsolationPerSession); err != nil {
		t.Fatalf("a manifest without sync is valid under any isolation: %v", err)
	}
}

func TestSyncSpecValidateRefusesPerSessionIsolation(t *testing.T) {
	// A per-session plugin's data root holds only per-session instance directories.
	s := &SyncSpec{Paths: []string{"state.json"}}
	if err := s.Validate(IsolationPerSession); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("sync on a per-session plugin: %v, want ErrInvalidManifest", err)
	}
}

func TestSyncSpecValidateCapsTheNumberOfPatterns(t *testing.T) {
	paths := make([]string, MaxSyncPatterns+1)
	for i := range paths {
		paths[i] = "f" + strings.Repeat("x", i)
	}
	if err := (&SyncSpec{Paths: paths[:MaxSyncPatterns]}).Validate(IsolationPerPlugin); err != nil {
		t.Fatalf("exactly MaxSyncPatterns patterns: %v", err)
	}
	if err := (&SyncSpec{Paths: paths}).Validate(IsolationPerPlugin); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("one pattern over the cap: %v, want ErrInvalidManifest", err)
	}
}

func TestSyncSpecMatches(t *testing.T) {
	s := &SyncSpec{Paths: []string{"state.json", "profiles/*.json", "notes/**", "**/settings.toml"}}
	cases := []struct {
		rel  string
		want bool
	}{
		{"state.json", true},
		{"state.json.bak", false},
		{"profiles/a.json", true},
		{"profiles/a.txt", false},
		{"profiles/deep/a.json", false},
		{"notes/a", true},
		{"notes/x/y/z.md", true},
		{"notes", false},
		{"settings.toml", true},
		{"a/b/settings.toml", true},
		{"other.json", false},
		{"notes/tmp/x", false},
		{"tmp/settings.toml", false},
		{"../state.json", false},
		{"/state.json", false},
		{"notes/a:ads", false},
		{`notes\a`, false},
		{"", false},
	}
	for _, c := range cases {
		if got := s.Matches(c.rel); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	var none *SyncSpec
	if none.Matches("state.json") {
		t.Error("a plugin that declares no sync paths syncs nothing")
	}
}

func TestValidSyncFile(t *testing.T) {
	for _, ok := range []string{"state.json", "a/b/c.json", "[weird] name.json"} {
		if err := ValidSyncFile(ok); err != nil {
			t.Errorf("ValidSyncFile(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "../x", "a/../../x", "/x", "C:/x", "x:y", `a\b`, "tmp/x", "a/./b", "aux.txt", "x\x00"} {
		if err := ValidSyncFile(bad); !errors.Is(err, ErrUnsafeSyncPath) {
			t.Errorf("ValidSyncFile(%q) = %v, want ErrUnsafeSyncPath", bad, err)
		}
	}
}

func TestManifestValidateChecksSync(t *testing.T) {
	m := Manifest{ID: "com.example.echo", Name: "Echo", Version: "1.0.0",
		Engine: EngineConfig{Type: EngineGoBinary, Entry: "echo.exe"}, Sync: &SyncSpec{Paths: []string{"../vault.age"}}}
	if err := m.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("a manifest declaring a traversing sync path: %v, want ErrInvalidManifest", err)
	}
}
