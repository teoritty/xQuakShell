package recovery

import (
	"context"
	"os"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/vault"
)

// writeLegacyVault puts a pre-envelope vault on disk: the data encrypted directly under the master
// password, which is what every installation written before recovery keys existed actually holds.
func writeLegacyVault(t *testing.T, dir, password string) {
	t.Helper()
	data := domain.NewVaultData()
	data.KnownHosts = []string{"legacy.example ssh-ed25519 AAAA"}

	ciphertext, err := vault.EncryptLegacy(data, password)
	if err != nil {
		t.Fatalf("encrypt legacy vault: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(vault.FilePath(dir), ciphertext, 0o600); err != nil {
		t.Fatalf("write legacy vault: %v", err)
	}
}

func openRepo(t *testing.T, dir string) *persistence.VaultRepo {
	t.Helper()
	return persistence.NewVaultRepo(dir)
}

func mustOpen(t *testing.T, dir, credential string) *domain.VaultData {
	t.Helper()
	_, data, _, err := vault.Open(dir, credential)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	return data
}

// An existing user unlocks with the password they have always used, and comes out the other side
// with a converted vault and a key to write down. Anything less and the people this feature was
// built for never get it.
func TestUnlockingAnOldVaultConvertsItAndOffersAKey(t *testing.T) {
	dir := t.TempDir()
	const password = "the-password-from-before"
	writeLegacyVault(t, dir, password)

	repo := openRepo(t, dir)
	method, err := repo.UnlockWithCredential(context.Background(), password)
	if err != nil {
		t.Fatalf("unlock a legacy vault: %v", err)
	}
	if method != domain.UnlockByPassword {
		t.Errorf("method = %v, want UnlockByPassword", method)
	}
	if !repo.ConvertedOnUnlock() {
		t.Fatal("the unlock did not report a conversion, so nothing would offer the user a key")
	}
	if repo.HasRecoveryKey() {
		t.Error("a converted vault already has a recovery key; nobody has been shown one")
	}

	data, err := repo.GetData()
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if len(data.KnownHosts) != 1 {
		t.Error("the conversion lost the vault contents")
	}

	key, err := repo.IssueRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("issue the first key: %v", err)
	}
	repo.Lock()

	if _, _, _, err := vault.Open(dir, domain.FormatRecoveryKey(key)); err != nil {
		t.Errorf("the key issued after conversion does not open the vault: %v", err)
	}
	if _, _, _, err := vault.Open(dir, password); err != nil {
		t.Errorf("the original password stopped working after conversion: %v", err)
	}
}

// The original bytes have to be kept before they are replaced. A conversion that rewrote the only
// copy of someone's keys with no way back is the one outcome this must never produce.
func TestConversionBacksUpBeforeItRewrites(t *testing.T) {
	dir := t.TempDir()
	const password = "the-password-from-before"
	writeLegacyVault(t, dir, password)

	original, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read the legacy vault: %v", err)
	}

	repo := openRepo(t, dir)
	if _, err := repo.UnlockWithCredential(context.Background(), password); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	repo.Lock()

	backup, err := os.ReadFile(vault.BackupPath(dir, domain.CurrentVaultVersion))
	if err != nil {
		t.Fatalf("no backup was taken: %v", err)
	}
	if string(backup) != string(original) {
		t.Error("the backup is not a byte copy of the vault as it stood before the conversion")
	}

	converted, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read the converted vault: %v", err)
	}
	if vault.IsLegacyAgeFile(converted) {
		t.Error("the vault is still in the old format after a reported conversion")
	}
}

// The conversion happens once. A second unlock must not report one, or the user is handed a fresh
// key on every launch and learns to click past the screen that must not be clicked past.
func TestASecondUnlockDoesNotReportAnotherConversion(t *testing.T) {
	dir := t.TempDir()
	const password = "the-password-from-before"
	writeLegacyVault(t, dir, password)

	first := openRepo(t, dir)
	if _, err := first.UnlockWithCredential(context.Background(), password); err != nil {
		t.Fatalf("first unlock: %v", err)
	}
	first.Lock()

	second := openRepo(t, dir)
	if _, err := second.UnlockWithCredential(context.Background(), password); err != nil {
		t.Fatalf("second unlock: %v", err)
	}
	if second.ConvertedOnUnlock() {
		t.Error("the second unlock reported a conversion; a key would be minted and shown on every launch")
	}
}

// A wrong password against a legacy vault must fail as a wrong password, not as a conversion
// failure or anything else that would look like the file is broken.
func TestAWrongPasswordAgainstAnOldVaultLeavesItAlone(t *testing.T) {
	dir := t.TempDir()
	writeLegacyVault(t, dir, "the-password-from-before")

	before, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	repo := openRepo(t, dir)
	if _, err := repo.UnlockWithCredential(context.Background(), "not-the-password"); err == nil {
		t.Fatal("a wrong password opened a legacy vault")
	}
	if repo.ConvertedOnUnlock() {
		t.Error("a refused unlock reported a conversion")
	}
	repo.Lock()

	after, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("a refused unlock rewrote the vault file")
	}
	if _, err := os.Stat(vault.BackupPath(dir, domain.CurrentVaultVersion)); err == nil {
		t.Error("a refused unlock took a backup, which will now block the real conversion's backup")
	}
}
