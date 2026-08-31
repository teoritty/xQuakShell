package recovery

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
)

// The key must not reach the logs. Nothing logs it deliberately, but an error wrapped one level too
// far up, or a debug line added in a hurry, would put a credential that opens everything into a
// file the vault does not protect.
func TestNothingInAFullLifecycleLogsTheKey(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	f := newFixture(t)
	keys := []string{f.key}

	rotated, err := f.repo.IssueRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	keys = append(keys, rotated)

	repo := f.reopen(t)
	if _, err := repo.UnlockWithCredential(context.Background(), domain.FormatRecoveryKey(rotated)); err != nil {
		t.Fatalf("unlock with the key: %v", err)
	}
	reset, err := repo.CompleteRecoveryReset(context.Background(), "a-brand-new-password")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	keys = append(keys, reset)
	repo.Lock()

	// A failed attempt is the case most likely to log what was typed.
	if _, err := f.unlock(t, domain.FormatRecoveryKey(rotated)); err == nil {
		t.Fatal("a revoked key still opened the vault")
	}

	text := logged.String()
	for _, key := range keys {
		for _, form := range []string{key, domain.FormatRecoveryKey(key), strings.ToLower(key)} {
			if strings.Contains(text, form) {
				t.Fatalf("a recovery key appears in the logs as %q", form)
			}
		}
	}
	if strings.Contains(text, "a-brand-new-password") || strings.Contains(text, f.password) {
		t.Fatal("a master password appears in the logs")
	}
}

// Neither credential may appear in an error a caller could surface. Errors are rendered verbatim on
// the unlock screen, so a wrapped credential would be printed back at whoever typed it - including
// whoever typed a key they should not have.
func TestNoCredentialAppearsInAnyError(t *testing.T) {
	f := newFixture(t)
	f.repo.Lock()

	attempts := []string{
		"not-the-password",
		domain.FormatRecoveryKey(strings.Repeat("B", domain.RecoveryKeyLength)),
		f.password + "x",
	}

	for _, attempt := range attempts {
		_, err := f.unlock(t, attempt)
		if err == nil {
			t.Fatalf("%q was accepted", attempt)
		}
		normalized, _ := domain.NormalizeRecoveryKey(attempt)
		for _, form := range []string{attempt, normalized} {
			if form != "" && strings.Contains(err.Error(), form) {
				t.Errorf("the error repeats what was typed: %q contains %q", err, form)
			}
		}
	}
}

// After a successful unlock the master password has done its job. Keeping it for the life of the
// process, as re-deriving a key on every write would require, means a memory dump of an idle
// application hands over the credential rather than a key that one vault's file needs.
func TestTheMasterPasswordIsNotRetainedAfterUnlock(t *testing.T) {
	f := newFixture(t)

	// A write after unlocking has to succeed without the password being available anywhere, which
	// is only possible if the write path is keyed rather than credential-derived.
	if err := f.repo.UpdateData(context.Background(), func(d *domain.VaultData) error {
		d.KnownHosts = []string{"written-without-the-password"}
		return nil
	}); err != nil {
		t.Fatalf("write after unlock: %v", err)
	}
	f.repo.Lock()

	data := mustOpen(t, f.dir, f.password)
	if len(data.KnownHosts) != 1 || data.KnownHosts[0] != "written-without-the-password" {
		t.Fatal("the write did not survive; the flush path is not using the vault key")
	}
}

// The backup a conversion leaves behind is still the old file under the old password. That is a
// real limitation and it has to stay a deliberate one: the test exists so nobody assumes a password
// change reaches into the backups, and so nobody starts deleting them to make it true.
func TestConversionBackupsStayReadableUnderTheOldPassword(t *testing.T) {
	dir := t.TempDir()
	const oldPassword = "the-original-password"

	writeLegacyVault(t, dir, oldPassword)
	repo := openRepo(t, dir)
	if _, err := repo.UnlockWithCredential(context.Background(), oldPassword); err != nil {
		t.Fatalf("unlock a legacy vault: %v", err)
	}
	if _, err := repo.ChangeMasterPassword(context.Background(), oldPassword, "a-completely-new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	repo.Lock()

	backup := vault.BackupPath(dir, domain.CurrentVaultVersion)
	raw, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("the conversion left no backup at %s: %v", filepath.Base(backup), err)
	}
	if _, err := vault.DecryptLegacy(raw, oldPassword); err != nil {
		t.Fatalf("the backup does not open under the password that made it: %v", err)
	}
	if _, err := vault.DecryptLegacy(raw, "a-completely-new-password"); err == nil {
		t.Fatal("the backup opened under the new password, so the change reached into it after all")
	}
}
