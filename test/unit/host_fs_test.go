package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/host"
	"ssh-client/internal/pkg/pathsafe"
)

func TestHostFSAllowsPathOutsidePortableRoot(t *testing.T) {
	fs := host.NewHostFS()
	outside := t.TempDir()
	nodes, err := fs.List(outside, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if nodes == nil {
		t.Fatal("expected empty slice, got nil")
	}
}

func TestHostFSDefaultPathIsUserHome(t *testing.T) {
	fs := host.NewHostFS()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	if fs.DefaultPath() != filepath.Clean(expected) {
		t.Fatalf("expected default path %q, got %q", expected, fs.DefaultPath())
	}
}

func TestHostFSEmptyPathResolvesToDefault(t *testing.T) {
	fs := host.NewHostFS()
	resolved, err := fs.ResolvePath("")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != fs.DefaultPath() {
		t.Fatalf("expected %q, got %q", fs.DefaultPath(), resolved)
	}
}

func TestHostFSRejectsNullBytePath(t *testing.T) {
	fs := host.NewHostFS()
	if _, err := fs.ResolvePath("bad\x00path"); err == nil {
		t.Fatal("expected null byte path to fail")
	}
	if _, err := fs.ResolvePath("bad\x00path"); err != domain.ErrHostPathInvalid {
		t.Fatalf("expected ErrHostPathInvalid, got %v", err)
	}
}

func TestResolveHostPathRejectsEmpty(t *testing.T) {
	if _, err := pathsafe.ResolveHostPath(""); err == nil {
		t.Fatal("expected empty path to fail")
	}
}

func TestHostFSCreateFileOutsidePortableRoot(t *testing.T) {
	fs := host.NewHostFS()
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	if err := fs.CreateFile(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestHostFSStatFile(t *testing.T) {
	fs := host.NewHostFS()
	dir := t.TempDir()
	target := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := fs.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir {
		t.Fatal("expected file, got directory")
	}
	if info.Size != 5 {
		t.Fatalf("expected size 5, got %d", info.Size)
	}
}

func TestHostFSStatDirectory(t *testing.T) {
	fs := host.NewHostFS()
	dir := t.TempDir()
	info, err := fs.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir {
		t.Fatal("expected directory")
	}
}
