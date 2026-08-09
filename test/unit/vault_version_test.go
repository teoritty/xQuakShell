package unit

import (
	"context"
	"errors"
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
	if err := vault.WriteVaultFile(dir, versionTestPassphrase, data); err != nil {
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
		{"older", domain.CurrentVaultVersion - 1, domain.ErrVaultVersionTooOld},
	}

	for _, tc := range cases {
		data := domain.NewVaultData()
		data.Version = tc.version
		ciphertext, err := vault.Encrypt(data, versionTestPassphrase)
		if err != nil {
			t.Fatalf("%s: encrypt: %v", tc.name, err)
		}
		if _, err := vault.Decrypt(ciphertext, versionTestPassphrase); !errors.Is(err, tc.want) {
			t.Errorf("%s vault (version %d): got %v, want %v; the two directions need opposite actions from the user", tc.name, tc.version, err, tc.want)
		}
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
