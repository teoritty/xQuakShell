package pathsafe_test

import (
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/pkg/pathsafe"
)

func TestSecurePathUnderRootsRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(allowed, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}

	_, err := pathsafe.SecurePathUnderRoots(link, []string{root})
	if err != pathsafe.ErrPathDenied {
		t.Fatalf("expected ErrPathDenied, got %v", err)
	}
}

func TestReadExistingFileUsesOpenValidation(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "note.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := pathsafe.ReadExistingFile([]string{root}, filePath, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content %q", data)
	}
}

func TestWriteExistingFileCreatesNewFile(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "new.txt")
	if err := pathsafe.WriteExistingFile([]string{root}, filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Fatalf("unexpected content %q", data)
	}
}
