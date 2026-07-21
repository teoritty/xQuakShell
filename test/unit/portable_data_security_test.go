package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/infra/portable"
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

func TestPortableDataStoreRemoveUnderRoot(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	target := filepath.Join(root, "to-delete")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected path removed, stat err=%v", err)
	}
}

func TestPortableDataStoreRemoveRejectsEscape(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(outside); err == nil {
		t.Fatal("expected remove outside root to fail")
	}
}

func TestPortableDataStoreReadFileUnderRoot(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	target := filepath.Join(root, "readme.txt")
	content := []byte("portable data")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := store.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("expected %q, got %q", content, data)
	}
}

func TestPortableDataStoreReadFileRejectsEscape(t *testing.T) {
	root := t.TempDir()
	store := portable.NewDataStore(root, "", nil)

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadFile(outside); err == nil {
		t.Fatal("expected read outside root to fail")
	}
}
