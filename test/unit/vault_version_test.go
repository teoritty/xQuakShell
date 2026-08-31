package unit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/vault"
)

const versionTestPassphrase = "test-master-password"

// writeVaultAtVersion puts a vault of an arbitrary schema version on disk. There are no binary
// vault fixtures in the repository on purpose — a fixture would freeze the age parameters and
// start failing for reasons that have nothing to do with the version gate.
func writeVaultAtVersion(t *testing.T, dir string, version int) []byte {
	t.Helper()
	data := domain.NewVaultData()
	data.Version = version
	session, err := vault.CreateSession(dir, versionTestPassphrase)
	if err != nil {
		t.Fatalf("create session at version %d: %v", version, err)
	}
	if err := session.Save(data); err != nil {
		t.Fatalf("write vault at version %d: %v", version, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "vault.age"))
	if err != nil {
		t.Fatalf("read back vault file: %v", err)
	}
	return raw
}

func TestVaultDecryptTellsANewerSchemaFromAnOlderOne(t *testing.T) {
	cases := []struct {
		name    string
		version int
		want    error
	}{
		{"newer", domain.CurrentVaultVersion + 1, domain.ErrVaultVersionTooNew},
		{"below the migratable floor", domain.MinMigratableVaultVersion - 1, domain.ErrVaultVersionTooOld},
	}

	for _, tc := range cases {
		dir := t.TempDir()
		writeVaultAtVersion(t, dir, tc.version)
		if _, _, _, err := vault.Open(dir, versionTestPassphrase); !errors.Is(err, tc.want) {
			t.Errorf("%s vault (version %d): got %v, want %v; the two directions need opposite actions from the user", tc.name, tc.version, err, tc.want)
		}
	}
}

// The same gate has to hold on the pre-envelope format, because that is what every installation
// written by an older build still has on disk.
func TestLegacyVaultVersionGatesMatchTheEnvelopeOnes(t *testing.T) {
	cases := []struct {
		version int
		want    error
	}{
		{domain.CurrentVaultVersion + 1, domain.ErrVaultVersionTooNew},
		{domain.MinMigratableVaultVersion - 1, domain.ErrVaultVersionTooOld},
	}

	for _, tc := range cases {
		data := domain.NewVaultData()
		data.Version = tc.version
		ciphertext, err := vault.EncryptLegacy(data, versionTestPassphrase)
		if err != nil {
			t.Fatalf("version %d: encrypt: %v", tc.version, err)
		}
		if _, err := vault.DecryptLegacy(ciphertext, versionTestPassphrase); !errors.Is(err, tc.want) {
			t.Errorf("legacy vault (version %d): got %v, want %v", tc.version, err, tc.want)
		}
	}
}

// A migratable vault must decrypt rather than be refused, and must come back carrying its own
// version. Decrypt returning the current version instead would make the migration invisible to
// every caller that decides whether to run one.
func TestVaultDecryptReturnsAMigratableSchemaUntouched(t *testing.T) {
	dir := t.TempDir()
	writeVaultAtVersion(t, dir, domain.MinMigratableVaultVersion)

	_, got, _, err := vault.Open(dir, versionTestPassphrase)
	if err != nil {
		t.Fatalf("decrypt a migratable vault: %v", err)
	}
	if got.Version != domain.MinMigratableVaultVersion {
		t.Errorf("version = %d, want %d; Decrypt must not silently upgrade what it reads", got.Version, domain.MinMigratableVaultVersion)
	}
	if !vault.NeedsMigration(got) {
		t.Error("NeedsMigration = false for an old schema; nothing downstream would ever run the migration")
	}
}

// The guarantee a rollback depends on: a build that meets data it cannot read must leave that data
// exactly as it found it. There is no explicit write guard — the property comes from Unlock
// refusing, so nothing is ever marked unlocked or dirty, and from Create refusing to overwrite an
// existing file. This test is what stops either of those from being loosened by accident.
func TestVaultFromANewerBuildIsNeverWrittenTo(t *testing.T) {
	dir := t.TempDir()
	before := writeVaultAtVersion(t, dir, domain.CurrentVaultVersion+1)

	repo := persistence.NewVaultRepo(dir)
	ctx := context.Background()

	if err := repo.Unlock(ctx, versionTestPassphrase); !errors.Is(err, domain.ErrVaultVersionTooNew) {
		t.Fatalf("unlock: got %v, want ErrVaultVersionTooNew", err)
	}
	if repo.IsUnlocked() {
		t.Error("repo reports unlocked after refusing a newer vault; every write path keys on this flag")
	}
	if err := repo.Create(ctx, versionTestPassphrase); !errors.Is(err, domain.ErrVaultAlreadyExists) {
		t.Fatalf("create over a newer vault: got %v, want ErrVaultAlreadyExists", err)
	}

	// Lock runs the flush path, which is where an unguarded write would land.
	repo.Lock()

	after, err := os.ReadFile(filepath.Join(dir, "vault.age"))
	if err != nil {
		t.Fatalf("read vault file: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the vault file changed after a refused unlock; a rollback to this build would now be losing whatever the newer build stored")
	}
}

func TestBackupVaultFileNamesTheVersionItLeavesBehind(t *testing.T) {
	dir := t.TempDir()
	original := writeVaultAtVersion(t, dir, domain.CurrentVaultVersion)

	if err := vault.BackupVaultFile(dir, domain.CurrentVaultVersion); err != nil {
		t.Fatalf("backup: %v", err)
	}

	backupPath := filepath.Join(dir, fmt.Sprintf("vault.age.v%d.bak", domain.CurrentVaultVersion))
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != string(original) {
		t.Error("the backup is not a byte copy of the vault it was taken from")
	}
}

// A second migration attempt must not replace the pre-migration copy with already-migrated bytes:
// that copy is the only remaining record of the data as it stood before any upgrade ran.
func TestBackupVaultFileRefusesToOverwriteAnEarlierBackup(t *testing.T) {
	dir := t.TempDir()
	writeVaultAtVersion(t, dir, domain.CurrentVaultVersion)
	if err := vault.BackupVaultFile(dir, domain.CurrentVaultVersion); err != nil {
		t.Fatalf("first backup: %v", err)
	}

	backupPath := filepath.Join(dir, fmt.Sprintf("vault.age.v%d.bak", domain.CurrentVaultVersion))
	first, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read first backup: %v", err)
	}

	// Stand in for "the vault has since been rewritten by a migration".
	rewritten := writeVaultAtVersion(t, dir, domain.CurrentVaultVersion)
	if string(rewritten) == string(first) {
		t.Fatal("the rewrite produced identical bytes, so this test cannot detect a clobber")
	}

	if err := vault.BackupVaultFile(dir, domain.CurrentVaultVersion); err != nil {
		t.Fatalf("second backup: %v", err)
	}
	second, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup after second call: %v", err)
	}
	if string(second) != string(first) {
		t.Error("the second call overwrote the original backup with post-migration bytes")
	}
}

func TestBackupVaultFileReportsAMissingVault(t *testing.T) {
	if err := vault.BackupVaultFile(t.TempDir(), 3); !errors.Is(err, domain.ErrVaultNotFound) {
		t.Errorf("got %v, want ErrVaultNotFound", err)
	}
}
