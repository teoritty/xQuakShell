package unit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/keys"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/vault"
)

func migrationDeps() persistence.MigrationDeps {
	return persistence.MigrationDeps{Codec: keys.NewCodec(), NewDataKey: keys.NewDataKey}
}

// v3Key produces the bytes a schema 3 vault held: an OpenSSH private key, protected or not,
// stored exactly as the user's file was. passphrase empty means an unprotected key.
func v3Key(t *testing.T, passphrase string) []byte {
	t.Helper()
	codec := keys.NewCodec()
	material, err := codec.Generate(domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519, Comment: "v3"}, []byte("scratch"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	out, err := codec.Export(material.PEM, []byte("scratch"), []byte(passphrase), "v3")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return out
}

// writeV3Vault puts a schema 3 vault on disk holding the given keys plus one connection, so a
// test can prove the migration carried the rest of the vault across too.
func writeV3Vault(t *testing.T, dir string, blobs map[string][]byte) {
	t.Helper()
	data := domain.NewVaultData()
	data.Version = 3
	for id, pemData := range blobs {
		data.Identities[id] = domain.SSHIdentity{ID: id, Comment: id, KeyType: "openssh"}
		data.KeyBlobs[id] = domain.IdentityBlob{PEMData: pemData}
	}
	data.Connections = append(data.Connections, domain.Connection{ID: "conn-1", Name: "prod", Host: "example.test", Port: 22})
	if err := vault.WriteVaultFile(dir, versionTestPassphrase, data); err != nil {
		t.Fatalf("write v3 vault: %v", err)
	}
}

func TestUnlockRefusesAnUnmigratedVaultWithoutTouchingIt(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"plain": v3Key(t, "")})
	before, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}

	repo := persistence.NewVaultRepo(dir)
	if err := repo.Unlock(context.Background(), versionTestPassphrase); !errors.Is(err, domain.ErrVaultMigrationRequired) {
		t.Fatalf("unlock a v3 vault: got %v, want ErrVaultMigrationRequired", err)
	}
	if repo.IsUnlocked() {
		t.Error("repo reports unlocked after refusing an unmigrated vault; every write path keys on this flag")
	}
	repo.Lock()

	after, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the vault file changed on a refused unlock; nothing may be written before the backup is taken")
	}
}

func TestPlanNamesOnlyTheKeysThatNeedAPassphrase(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{
		"plain":     v3Key(t, ""),
		"protected": v3Key(t, "hunter2"),
	})

	repo := persistence.NewVaultRepo(dir)
	pending, err := repo.PlanMigration(context.Background(), versionTestPassphrase, migrationDeps())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "protected" {
		t.Fatalf("pending = %+v, want exactly the protected key; asking for a passphrase the key does not have wastes the user's time, and missing one strands the key", pending)
	}
	if repo.IsUnlocked() {
		t.Error("planning left the repo unlocked; a plan must not open the vault for use")
	}
}

func TestMigrationBacksUpBeforeItWritesAndKeepsEverythingElse(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"plain": v3Key(t, "")})
	original, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}

	repo := persistence.NewVaultRepo(dir)
	report, err := repo.CompleteMigration(context.Background(), versionTestPassphrase, nil, migrationDeps())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backup, err := os.ReadFile(filepath.Join(dir, "vault.age.v3.bak"))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != string(original) {
		t.Error("the backup does not hold the pre-migration bytes; it is the only way back and must be taken before the first write")
	}
	if report.BackupPath == "" {
		t.Error("report carries no backup path; the user cannot act on a backup they cannot find")
	}
	if len(report.Converted) != 1 || len(report.Skipped) != 0 {
		t.Errorf("report = %+v, want one converted and none skipped", report)
	}

	data, err := repo.GetData()
	if err != nil {
		t.Fatalf("get data after migration: %v", err)
	}
	if data.Version != domain.CurrentVaultVersion {
		t.Errorf("version after migration = %d, want %d", data.Version, domain.CurrentVaultVersion)
	}
	if len(data.Connections) != 1 || data.Connections[0].ID != "conn-1" {
		t.Errorf("connections after migration = %+v; the migration must carry the whole vault, not just the keys", data.Connections)
	}
}

func TestMigrationRewrapsAnUnprotectedKeyUnderTheVault(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"plain": v3Key(t, "")})

	repo := persistence.NewVaultRepo(dir)
	if _, err := repo.CompleteMigration(context.Background(), versionTestPassphrase, nil, migrationDeps()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	data, err := repo.GetData()
	if err != nil {
		t.Fatalf("get data: %v", err)
	}

	identity := data.Identities["plain"]
	blob := data.KeyBlobs["plain"]
	if identity.Policy != domain.KeyPolicyVault {
		t.Errorf("policy = %q, want %q", identity.Policy, domain.KeyPolicyVault)
	}
	if len(blob.DataKey) == 0 {
		t.Fatal("no data key stored for a vault-policy identity; the key would be unusable")
	}
	if _, err := gossh.ParsePrivateKey(blob.PEMData); err == nil {
		t.Error("the migrated key still parses with no passphrase; an unprotected v3 key must not stay unprotected")
	}
	if _, err := keys.NewCodec().Unwrap(blob.PEMData, blob.DataKey); err != nil {
		t.Errorf("stored data key does not open the migrated key: %v", err)
	}
	if identity.Fingerprint == "" || identity.PublicKey == "" {
		t.Error("migration left the identity without a fingerprint or public key; the manager derives both without unwrapping and cannot show a key that has neither")
	}
}

func TestMigrationRewrapsAProtectedKeyUnderTheUsersPassphrase(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"protected": v3Key(t, "hunter2")})

	repo := persistence.NewVaultRepo(dir)
	report, err := repo.CompleteMigration(context.Background(), versionTestPassphrase,
		map[string]string{"protected": "hunter2"}, migrationDeps())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(report.Converted) != 1 {
		t.Fatalf("report = %+v, want the key converted", report)
	}

	data, _ := repo.GetData()
	identity := data.Identities["protected"]
	blob := data.KeyBlobs["protected"]
	if identity.Policy != domain.KeyPolicyPassphrase {
		t.Errorf("policy = %q, want %q; migration must not move a key out of the user's control", identity.Policy, domain.KeyPolicyPassphrase)
	}
	if len(blob.DataKey) != 0 {
		t.Error("a data key was stored for a passphrase-policy identity; the vault would then hold something that opens the key without the user, defeating the policy entirely")
	}
	if _, err := keys.NewCodec().Unwrap(blob.PEMData, []byte("hunter2")); err != nil {
		t.Errorf("the migrated key does not open with the passphrase the user supplied: %v", err)
	}
}

func TestASkippedKeyKeepsWorkingAndTheVaultStillOpens(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{
		"forgotten": v3Key(t, "lost-forever"),
		"plain":     v3Key(t, ""),
	})
	repo := persistence.NewVaultRepo(dir)

	// No answer for "forgotten": the user could not remember it. The whole vault must still open.
	report, err := repo.CompleteMigration(context.Background(), versionTestPassphrase, nil, migrationDeps())
	if err != nil {
		t.Fatalf("migrate with a skipped key: %v", err)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "forgotten" {
		t.Errorf("skipped = %v, want exactly the forgotten key", report.Skipped)
	}

	data, err := repo.GetData()
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if !data.Identities["forgotten"].MigrationPending {
		t.Error("a skipped key is not marked pending; the manager would show it as finished and never offer to retry it")
	}
	if !data.KeyBlobs["forgotten"].Legacy {
		t.Error("a skipped key's blob is not marked legacy; the read path would treat unconverted bytes as converted ones")
	}
	if _, err := keys.NewCodec().Unwrap(data.KeyBlobs["forgotten"].PEMData, []byte("lost-forever")); err != nil {
		t.Errorf("a skipped key no longer opens with its own passphrase: %v; skipping must leave the bytes exactly as they were", err)
	}
	if data.Identities["plain"].MigrationPending {
		t.Error("one skipped key marked its neighbour pending too")
	}
}

func TestAWrongPassphraseSkipsThatKeyInsteadOfFailingTheUpgrade(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"protected": v3Key(t, "right")})

	repo := persistence.NewVaultRepo(dir)
	report, err := repo.CompleteMigration(context.Background(), versionTestPassphrase,
		map[string]string{"protected": "wrong"}, migrationDeps())
	if err != nil {
		t.Fatalf("a wrong passphrase failed the whole migration: %v; that leaves the user with no way into any of their data", err)
	}
	if len(report.Skipped) != 1 {
		t.Errorf("report = %+v, want the key skipped so the user can retry it later", report)
	}
}

func TestASecondMigrationDoesNotReplaceTheOriginalBackup(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"plain": v3Key(t, "")})
	repo := persistence.NewVaultRepo(dir)
	if _, err := repo.CompleteMigration(context.Background(), versionTestPassphrase, nil, migrationDeps()); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	firstBackup, err := os.ReadFile(filepath.Join(dir, "vault.age.v3.bak"))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}

	// The vault is now at the current version, so a second attempt has nothing to migrate.
	if _, err := repo.CompleteMigration(context.Background(), versionTestPassphrase, nil, migrationDeps()); err == nil {
		t.Error("migrating an already-migrated vault reported success; it would take a backup of migrated bytes over the original")
	}

	secondBackup, err := os.ReadFile(filepath.Join(dir, "vault.age.v3.bak"))
	if err != nil {
		t.Fatalf("read backup after second attempt: %v", err)
	}
	if string(firstBackup) != string(secondBackup) {
		t.Error("the backup was overwritten; it is the last copy of the pre-migration data and a retry must never replace it")
	}
}

func TestMigrationRefusesAWrongMasterPassword(t *testing.T) {
	dir := t.TempDir()
	writeV3Vault(t, dir, map[string][]byte{"plain": v3Key(t, "")})

	repo := persistence.NewVaultRepo(dir)
	if _, err := repo.CompleteMigration(context.Background(), "not-the-master-password", nil, migrationDeps()); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("migrate with a wrong master password: got %v, want ErrVaultDecryptFailed", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault.age.v3.bak")); !os.IsNotExist(err) {
		t.Error("a backup was taken for a migration that could not decrypt anything")
	}
	if repo.IsUnlocked() {
		t.Error("a failed migration left the repo unlocked")
	}
}
