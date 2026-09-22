package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain"
)

// A directory deleted while the local pane was showing it must come back as ErrDirectoryNotFound,
// on every platform: Windows reports ERROR_PATH_NOT_FOUND, Unix ENOENT, and the pane recognises
// neither by itself.
func TestListOfADeletedDirectoryIsDirectoryNotFound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "doomed")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	_, err := NewHostFS().List(dir, true, nil)
	if !errors.Is(err, domain.ErrDirectoryNotFound) {
		t.Fatalf("List(deleted dir) = %v, want ErrDirectoryNotFound", err)
	}

	_, err = NewHostFS().List(filepath.Join(dir, "deeper"), true, nil)
	if !errors.Is(err, domain.ErrDirectoryNotFound) {
		t.Fatalf("List(under a deleted dir) = %v, want ErrDirectoryNotFound", err)
	}
}

func TestListOfAnExistingDirectoryStillLists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := NewHostFS().List(dir, true, nil)
	if err != nil || len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("List = (%+v, %v), want the one file", entries, err)
	}
}
