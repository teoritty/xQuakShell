package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExtractedTree lays out files the way the release extractors do: every entry carries the
// execute bit, because a zip written on Windows carries none and the extractor adds it.
func writeExtractedTree(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir for %q: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(name), 0o700); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	return dir
}

func TestFindEntryExecutablePicksTheManifestEntry(t *testing.T) {
	// README sorts first and is executable like everything else the extractor writes, so a
	// mode-based search would install it as the plugin.
	dir := writeExtractedTree(t, "README.md", "LICENSE", "xqs-vnc")

	got, err := findEntryExecutable(dir, "xqs-vnc", "xqs-vnc-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got) != "xqs-vnc" {
		t.Fatalf("picked %q, want the manifest entry xqs-vnc", got)
	}
}

func TestFindEntryExecutableAcceptsExeAlternate(t *testing.T) {
	cases := []struct {
		name      string
		files     []string
		entry     string
		wantBase  string
		assetName string
	}{
		{
			name:      "manifest names the exe, archive ships the posix binary",
			files:     []string{"docs/readme.txt", "xqs-vnc"},
			entry:     "xqs-vnc.exe",
			wantBase:  "xqs-vnc",
			assetName: "xqs-vnc-linux-amd64.tar.gz",
		},
		{
			name:      "manifest names the posix binary, archive ships the exe",
			files:     []string{"docs/readme.txt", "xqs-vnc.exe"},
			entry:     "xqs-vnc",
			wantBase:  "xqs-vnc.exe",
			assetName: "xqs-vnc-windows-amd64.zip",
		},
		{
			name:      "nested entry is matched by file name",
			files:     []string{"bin/xqs-vnc.exe"},
			entry:     "bin/xqs-vnc.exe",
			wantBase:  "xqs-vnc.exe",
			assetName: "xqs-vnc-windows-amd64.zip",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeExtractedTree(t, tc.files...)
			got, err := findEntryExecutable(dir, tc.entry, tc.assetName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if filepath.Base(got) != tc.wantBase {
				t.Fatalf("picked %q, want %q", filepath.Base(got), tc.wantBase)
			}
		})
	}
}

func TestFindEntryExecutableFailsWhenTheEntryIsAbsent(t *testing.T) {
	dir := writeExtractedTree(t, "README.md", "install.sh")

	_, err := findEntryExecutable(dir, "xqs-vnc", "xqs-vnc-linux-amd64.tar.gz")
	if err == nil {
		t.Fatal("expected an error when the archive holds no entry binary")
	}
	for _, want := range []string{"xqs-vnc", "xqs-vnc-linux-amd64.tar.gz"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected the error to name %q, got %q", want, err.Error())
		}
	}
}

func TestFindEntryExecutableRequiresAnEntryName(t *testing.T) {
	dir := writeExtractedTree(t, "xqs-vnc")

	if _, err := findEntryExecutable(dir, "  ", "xqs-vnc-linux-amd64.tar.gz"); err == nil {
		t.Fatal("expected an error when the manifest declares no engine.entry")
	}
}

func TestEntryNameCandidates(t *testing.T) {
	cases := []struct {
		entry string
		want  []string
	}{
		{entry: "xqs-vnc", want: []string{"xqs-vnc", "xqs-vnc.exe"}},
		{entry: "xqs-vnc.exe", want: []string{"xqs-vnc.exe", "xqs-vnc"}},
		{entry: "xqs-vnc.EXE", want: []string{"xqs-vnc.EXE", "xqs-vnc"}},
		{entry: "bin/xqs-vnc", want: []string{"bin/xqs-vnc", "bin/xqs-vnc.exe"}},
		{entry: "   ", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			got := entryNameCandidates(tc.entry)
			if len(got) != len(tc.want) {
				t.Fatalf("entryNameCandidates(%q) = %v, want %v", tc.entry, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("entryNameCandidates(%q) = %v, want %v", tc.entry, got, tc.want)
				}
			}
		})
	}
}
