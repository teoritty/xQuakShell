package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/infra/portable"
)

func TestPortableDataStoreRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ResolvePath(outside); err == nil {
		t.Fatal("expected resolve outside root to fail")
	}
}

func TestPortableDataStoreAllowsPathUnderRoot(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	sub := filepath.Join(root, "nested", "file.txt")
	if _, err := store.ResolvePath(sub); err != nil {
		t.Fatal(err)
	}
}

func TestPortableDataStoreEmptyPathReturnsRoot(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	resolved, err := store.ResolvePath("")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != store.DataRoot() {
		t.Fatalf("expected data root %q, got %q", store.DataRoot(), resolved)
	}
}
