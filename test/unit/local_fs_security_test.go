package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/infra/portable"
)

func TestLocalFSRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	fs := portable.NewLocalFS(root, "", nil)

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.List(outside, true, nil); err == nil {
		t.Fatal("expected list outside root to fail")
	}
	if err := fs.Remove(outside); err == nil {
		t.Fatal("expected remove outside root to fail")
	}
}

func TestLocalFSAllowsPathUnderRoot(t *testing.T) {
	root := t.TempDir()
	fs := portable.NewLocalFS(root, "", nil)

	sub := filepath.Join(root, "nested")
	if err := fs.Mkdir(sub); err != nil {
		t.Fatal(err)
	}
	nodes, err := fs.List(sub, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected empty dir, got %d entries", len(nodes))
	}
}

func TestLocalFSCreateFileUnderRoot(t *testing.T) {
	root := t.TempDir()
	fs := portable.NewLocalFS(root, "", nil)

	target := filepath.Join(root, "notes.txt")
	if err := fs.CreateFile(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}
