package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fixtureRoot is a repository with the files an archive ships.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"LICENSE":                         "license",
		"docs/sync-server.md":             "# guide",
		"packaging/linux/xqsyncd.service": "[Unit]",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// stubBuild stands in for go build: it writes a file where the binary would be.
func stubBuild(_ string, tg target, out string) error {
	return os.WriteFile(out, []byte("binary for "+tg.goos+"/"+tg.goarch), 0o644)
}

func TestPackShipsOneArchivePerTarget(t *testing.T) {
	root, out := fixtureRoot(t), t.TempDir()
	names, err := pack(root, "v1.5.0", out, stubBuild)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"xqsyncd-v1.5.0-linux-amd64.tar.gz", "xqsyncd-v1.5.0-linux-arm64.tar.gz", "xqsyncd-v1.5.0-windows-amd64.zip"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Fatalf("archives = %v, want %v (spec 9.5: windows-amd64, linux-amd64, linux-arm64)", names, want)
	}
}

func TestLinuxArchiveCarriesTheUnitAndAnExecutableBinary(t *testing.T) {
	root, out := fixtureRoot(t), t.TempDir()
	if _, err := pack(root, "v1.5.0", out, stubBuild); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(out, "xqsyncd-v1.5.0-linux-arm64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	modes := map[string]int64{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		modes[h.Name] = h.Mode
	}
	dir := "xqsyncd-v1.5.0-linux-arm64/"
	for _, name := range []string{"xqsyncd", "xqsyncd.service", "README.md", "LICENSE"} {
		if _, ok := modes[dir+name]; !ok {
			t.Errorf("%s missing from the archive (got %v); it unpacks into one directory named after itself", dir+name, modes)
		}
	}
	if modes[dir+"xqsyncd"]&0o111 == 0 {
		t.Error("the binary is not executable in the archive")
	}
}

func TestWindowsArchiveCarriesTheExeAndTheGuide(t *testing.T) {
	root, out := fixtureRoot(t), t.TempDir()
	if _, err := pack(root, "v1.5.0", out, stubBuild); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(filepath.Join(out, "xqsyncd-v1.5.0-windows-amd64.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	dir := "xqsyncd-v1.5.0-windows-amd64/"
	want := []string{dir + "LICENSE", dir + "README.md", dir + "xqsyncd.exe"}
	if !slices.Equal(names, want) {
		t.Fatalf("zip entries = %v, want %v", names, want)
	}
}

func TestPackRefusesAnUnsafeLabel(t *testing.T) {
	for _, label := range []string{"", "../x", "v1/2", "a b"} {
		if _, err := pack(fixtureRoot(t), label, t.TempDir(), stubBuild); err == nil {
			t.Errorf("label %q accepted; it becomes a file name", label)
		}
	}
}
