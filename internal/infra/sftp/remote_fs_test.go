package sftp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

	hostileName := `..\..\evil.exe`

	// Derive the escape target from the exact same join logic the
	// vulnerable code uses (filepath.Join on an absolute root followed by
	// filepath.Clean via Join's normalization), rather than hand-computing
	// a level count. On Windows, filepath.Join cleans `..\..\` against the
	// absolute root, walking up two directory levels from localDir.
	absLocalRoot, err := filepath.Abs(localDir)
	if err != nil {
		t.Fatalf("resolve abs localDir: %v", err)
	}
	outsideMarker := filepath.Join(absLocalRoot, hostileName)
	if strings.HasPrefix(outsideMarker, absLocalRoot) {
		t.Fatalf("test setup error: derived escape target %q is still under root %q; hostile name no longer traverses", outsideMarker, absLocalRoot)
	}
	_ = os.Remove(outsideMarker) // ensure clean slate

	// The entry is a directory, not a file: a directory-typed hostile entry
	// is handled purely with os.MkdirAll (no *sftp.Client involved), so a
	// containment regression surfaces as a genuine assertion failure below
	// rather than as a nil-pointer panic on fs.client.Open (fs.client is nil
	// in this unit test). The mock only answers for the top-level listing so
	// a broken containment check can't recurse into the escaped directory
	// forever.
	fs := &RemoteFS{
		readDirFn: func(dir string) ([]os.FileInfo, error) {
			if dir != "/remote" {
				return nil, nil
			}
			return []os.FileInfo{
				fakeFileInfo{name: hostileName, isDir: true, size: 4},
			}, nil
		},
	}

	var totalDone int64
	err = fs.downloadRecursive(context.Background(), "/remote", localDir, &totalDone, -1, nil)
	if err != nil {
		t.Fatalf("downloadRecursive returned error: %v", err)
	}

	if _, statErr := os.Stat(outsideMarker); !os.IsNotExist(statErr) {
		t.Fatalf("hostile entry escaped download root: %s exists (err=%v)", outsideMarker, statErr)
	}

	// Also confirm nothing was written anywhere along the parent chain
	// between localDir and the escape target (defence against a partial
	// containment regression that stops one level short of the full
	// traversal).
	for dir := filepath.Dir(outsideMarker); dir != absLocalRoot && len(dir) >= len(filepath.VolumeName(absLocalRoot)); dir = filepath.Dir(dir) {
		if _, statErr := os.Stat(filepath.Join(dir, "evil.exe")); !os.IsNotExist(statErr) {
			t.Fatalf("hostile entry wrote into parent chain at %s (err=%v)", dir, statErr)
		}
		if dir == filepath.Dir(dir) {
			break // reached filesystem root
		}
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
