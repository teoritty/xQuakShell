package sftp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSafeEntryName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"..", false},
		{"../..", false},
		{`..\..\evil.exe`, false},
		{"a/b", false},
		{"c:evil", false},
		{"name:stream", false},
		{"\x00", false},
		{"", false},
		{".", false},
		{"file.txt", true},
		{"..data", true}, // legitimate leading-dot name, not the literal ".." segment
		{"пример.txt", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := safeEntryName(c.name)
			if got != c.want {
				t.Errorf("safeEntryName(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// fakeFileInfo is a minimal os.FileInfo used to drive a fake directory
// listing without a real *sftp.Client.
type fakeFileInfo struct {
	name  string
	isDir bool
	size  int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

// TestDownloadRecursiveRejectsHostileEntryName drives downloadRecursive with a
// fake remote directory listing containing a Windows-traversal name. It
// asserts that nothing is created outside the temp download root: the seam
// (readDirFn) lets us exercise the real filtering + mkdir/download logic
// without a live SFTP server.
func TestDownloadRecursiveRejectsHostileEntryName(t *testing.T) {
	localDir := t.TempDir()
	outsideMarker := filepath.Join(filepath.Dir(localDir), "evil.exe")
	_ = os.Remove(outsideMarker) // ensure clean slate

	hostileName := `..\..\evil.exe`
	fs := &RemoteFS{
		readDirFn: func(dir string) ([]os.FileInfo, error) {
			return []os.FileInfo{
				fakeFileInfo{name: hostileName, isDir: false, size: 4},
			}, nil
		},
	}

	var totalDone int64
	err := fs.downloadRecursive(context.Background(), "/remote", localDir, &totalDone, -1, nil)
	if err != nil {
		t.Fatalf("downloadRecursive returned error: %v", err)
	}

	if _, statErr := os.Stat(outsideMarker); !os.IsNotExist(statErr) {
		t.Fatalf("hostile entry escaped download root: %s exists (err=%v)", outsideMarker, statErr)
	}

	entries, err := os.ReadDir(localDir)
	if err != nil {
		t.Fatalf("reading localDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected localDir to remain empty, got %d entries", len(entries))
	}
}

// TestDownloadRecursiveRejectsDotDotEntryName covers the case where the
// server-supplied name is the literal ".." segment, which pkg/sftp's own
// filter (client.go) does not always catch (it compares the raw wire
// filename before path.Base is applied).
func TestDownloadRecursiveRejectsDotDotEntryName(t *testing.T) {
	localDir := t.TempDir()
	fs := &RemoteFS{
		readDirFn: func(dir string) ([]os.FileInfo, error) {
			return []os.FileInfo{
				fakeFileInfo{name: "..", isDir: true, size: 0},
			}, nil
		},
	}

	var totalDone int64
	err := fs.downloadRecursive(context.Background(), "/remote", localDir, &totalDone, -1, nil)
	if err != nil {
		t.Fatalf("downloadRecursive returned error: %v", err)
	}

	entries, err := os.ReadDir(localDir)
	if err != nil {
		t.Fatalf("reading localDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected localDir to remain empty, got %d entries", len(entries))
	}
}
