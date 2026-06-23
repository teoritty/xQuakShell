package pathsafe_test

import (
	"path/filepath"
	"testing"

	"ssh-client/internal/pkg/pathsafe"
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

func TestResolveUnderRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	_, err := pathsafe.ResolveUnderRoot(root, "../outside")
	if err == nil {
		t.Fatal("expected escape rejection")
	}
}
