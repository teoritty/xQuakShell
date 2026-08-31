package recovery

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/vault"
)

// Every unlock in this package costs a full scrypt pass at log2N=18, roughly 256 MiB and a
// noticeable fraction of a second. Fixtures are therefore built once per test and reused rather
// than rebuilt per assertion; a suite that opened a vault per case would take minutes.
type fixture struct {
	dir      string
	password string
	key      string
	repo     *persistence.VaultRepo
}

// newFixture creates a vault with a password and a recovery key, left unlocked.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	repo := persistence.NewVaultRepo(dir)
	password := "correct-horse-battery-staple"

	if err := repo.Create(context.Background(), password); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	key, err := repo.IssueRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("issue recovery key: %v", err)
	}
	return &fixture{dir: dir, password: password, key: key, repo: repo}
}

// reopen locks the vault and returns a fresh repository over the same directory, which is what a
// restart looks like from the vault's point of view.
func (f *fixture) reopen(t *testing.T) *persistence.VaultRepo {
	t.Helper()
	f.repo.Lock()
	return persistence.NewVaultRepo(f.dir)
}

// unlock attempts a credential against a fresh repository and returns the method and error.
func (f *fixture) unlock(t *testing.T, credential string) (domain.UnlockMethod, error) {
	t.Helper()
	return f.reopen(t).UnlockWithCredential(context.Background(), credential)
}

// rawEnvelope reads the vault file as the generic JSON it is on disk, so a test can tamper with it
// the way someone holding the file would.
func rawEnvelope(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(vault.FilePath(dir))
	if err != nil {
		t.Fatalf("read vault file: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("the vault file is not the JSON envelope this suite tampers with: %v", err)
	}
	return env
}

// writeEnvelope puts a tampered envelope back on disk.
func writeEnvelope(t *testing.T, dir string, env map[string]any) {
	t.Helper()
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}
	if err := os.WriteFile(vault.FilePath(dir), raw, 0o600); err != nil {
		t.Fatalf("write tampered envelope: %v", err)
	}
}

// wraps returns the envelope's wrap list, failing the test if its shape is not what the tampering
// helpers assume.
func wraps(t *testing.T, env map[string]any) []any {
	t.Helper()
	list, ok := env["wraps"].([]any)
	if !ok {
		t.Fatalf("envelope has no wraps list: %#v", env["wraps"])
	}
	return list
}

// dropWrap removes the wrap of the given kind, standing in for someone editing the file to delete a
// credential they do not have.
func dropWrap(t *testing.T, env map[string]any, kind string) {
	t.Helper()
	kept := []any{}
	for _, w := range wraps(t, env) {
		entry, ok := w.(map[string]any)
		if !ok || entry["kind"] == kind {
			continue
		}
		kept = append(kept, w)
	}
	env["wraps"] = kept
}
