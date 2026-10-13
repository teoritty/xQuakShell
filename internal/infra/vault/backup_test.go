package vault_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
)

func TestBackupCopiesTheVaultUnderAReasonedName(t *testing.T) {
	dir := t.TempDir()
	original := []byte("age-encrypted vault bytes")
	if err := os.WriteFile(vault.FilePath(dir), original, 0o600); err != nil {
		t.Fatal(err)
	}
	b := vault.Backups{Dir: dir, Now: func() time.Time { return time.UnixMilli(1234) }}
	name, err := b.BackupNow(context.Background(), "sync-join")
	if err != nil {
		t.Fatal(err)
	}
	if name != "vault.age.sync-join-1234.bak" {
		t.Fatalf("name = %q; the user must be able to tell what the copy is from its name", name)
	}
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("backup = %q, %v", got, err)
	}
	if info, _ := os.Stat(filepath.Join(dir, name)); info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("backup mode %v is readable by others", info.Mode().Perm())
	}
}

func TestBackupNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(vault.FilePath(dir), []byte("v1"), 0o600)
	b := vault.Backups{Dir: dir, Now: func() time.Time { return time.UnixMilli(1) }}
	name, err := b.BackupNow(context.Background(), "sync-join")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(vault.FilePath(dir), []byte("v2"), 0o600)
	if _, err := b.BackupNow(context.Background(), "sync-join"); err == nil {
		t.Fatal("a second backup at the same instant replaced the first")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != "v1" {
		t.Fatalf("the earlier backup now holds %q", got)
	}
}

func TestBackupRefusesUnsafeReasonsAndMissingVaults(t *testing.T) {
	dir := t.TempDir()
	b := vault.Backups{Dir: dir, Now: time.Now}
	if _, err := b.BackupNow(context.Background(), "sync-join"); !errors.Is(err, domain.ErrVaultNotFound) {
		t.Fatalf("missing vault: %v", err)
	}
	_ = os.WriteFile(vault.FilePath(dir), []byte("v"), 0o600)
	for _, reason := range []string{"", "../evil", "a/b", "UPPER", "a b", "x.bak"} {
		if _, err := b.BackupNow(context.Background(), reason); err == nil {
			t.Errorf("reason %q accepted; it becomes part of a file name", reason)
		}
	}
}
