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

// The decoy attack this closes: an archive carrying both a/xqs-vnc and bin/xqs-vnc used to
// install a/xqs-vnc, because the search matched on base name across the whole tree and took
// whichever filepath.Walk reached first - and "a" sorts before "bin". Whoever writes the archive
// picks the name that sorts first, and their file is the one that gets +x and gets spawned.
func TestFindEntryExecutableIgnoresADecoyEarlierInWalkOrder(t *testing.T) {
	dir := writeExtractedTree(t, "a/xqs-vnc", "bin/xqs-vnc")

	got, err := findEntryExecutable(dir, "bin/xqs-vnc", "xqs-vnc-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("bin", "xqs-vnc")
	if !strings.HasSuffix(got, want) {
		t.Fatalf("picked %q, want the declared path %q; a decoy sorting earlier must not win", got, want)
	}
}

// A file with the right name at the wrong path is not the entry. Accepting it is what turned the
// declared path into a hint, and a hint is not a containment check.
func TestFindEntryExecutableRefusesAMatchAtAnotherPath(t *testing.T) {
	dir := writeExtractedTree(t, "vendor/deep/xqs-vnc")

	if _, err := findEntryExecutable(dir, "bin/xqs-vnc", "xqs-vnc-linux-amd64.tar.gz"); err == nil {
		t.Fatal("expected an error: the entry exists only at a path the manifest does not declare")
	}
}

// `tar czf x.tgz myplugin-1.2.0/` is how release tarballs are usually built, and engine.entry is
// relative to the plugin rather than to that wrapper.
func TestFindEntryExecutableLooksThroughASoleWrapperDirectory(t *testing.T) {
	dir := writeExtractedTree(t, "xqs-vnc-1.2.0/bin/xqs-vnc", "xqs-vnc-1.2.0/README.md")

	got, err := findEntryExecutable(dir, "bin/xqs-vnc", "xqs-vnc-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("xqs-vnc-1.2.0", "bin", "xqs-vnc")) {
		t.Fatalf("picked %q, want the entry inside the wrapper directory", got)
	}
}

// Two entries at the root mean this is not the wrapper shape, and picking one of them would be
// the base-name search coming back in through a side door.
func TestFindEntryExecutableDoesNotGuessAmongSeveralTopLevelEntries(t *testing.T) {
	dir := writeExtractedTree(t, "real-1.2.0/bin/xqs-vnc", "evil/bin/xqs-vnc")

	if _, err := findEntryExecutable(dir, "bin/xqs-vnc", "xqs-vnc-linux-amd64.tar.gz"); err == nil {
		t.Fatal("expected an error: with two top-level directories there is no unambiguous wrapper")
	}
}

// engine.entry reaches this function straight from a downloaded manifest, so it is attacker input.
func TestFindEntryExecutableRefusesATraversingEntry(t *testing.T) {
	dir := writeExtractedTree(t, "bin/xqs-vnc")

	for _, entry := range []string{"../outside", "bin/../../outside", "/etc/passwd"} {
		t.Run(entry, func(t *testing.T) {
			if _, err := findEntryExecutable(dir, entry, "asset.tar.gz"); err == nil {
				t.Fatalf("engine.entry %q escaped the archive root without an error", entry)
			}
		})
	}
}

// A directory named like the entry is not an executable, and returning it would fail much later
// at spawn with an error naming nothing useful.
func TestFindEntryExecutableRefusesADirectoryAtTheEntryPath(t *testing.T) {
	dir := writeExtractedTree(t, "bin/xqs-vnc/placeholder")

	if _, err := findEntryExecutable(dir, "bin/xqs-vnc", "asset.tar.gz"); err == nil {
		t.Fatal("expected an error: the declared entry path is a directory")
	}
}
