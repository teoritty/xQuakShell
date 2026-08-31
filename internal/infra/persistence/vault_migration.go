package persistence

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
	"xquakshell/internal/pkg/safego"
)

// PlanMigration decrypts the vault and reports which keys need a passphrase before it can be
// upgraded, without writing anything.
//
// It is a separate call from CompleteMigration so the wizard can collect every passphrase in one
// pass. The alternative — asking mid-rewrite — would leave a half-migrated vault on disk if the
// user closed the window at the wrong moment.
func (r *VaultRepo) PlanMigration(_ context.Context, masterPassword string, deps domain.MigrationDeps) (*domain.MigrationPlan, error) {
	r.mu.RLock()
	dir := r.dir
	r.mu.RUnlock()

	_, data, _, err := vault.Open(dir, masterPassword)
	if err != nil {
		return nil, err
	}
	defer releaseScryptPages()

	if !vault.NeedsMigration(data) {
		return &domain.MigrationPlan{}, nil
	}
	return &domain.MigrationPlan{Required: true, Keys: vault.PlanMigration(data, deps.Codec)}, nil
}

// CompleteMigration upgrades the vault on disk and leaves the repository unlocked on success.
//
// The backup is taken before anything is written and a failure to take it aborts the whole
// upgrade: a migration that rewrites the only copy of someone's keys with no way back is the one
// outcome this must never produce. BackupVaultFile keeps any earlier backup, so a second attempt
// after a crash cannot replace the original with already-migrated bytes.
//
// answers maps identity ID to passphrase; an ID the user skipped is simply absent.
func (r *VaultRepo) CompleteMigration(_ context.Context, masterPassword string, answers map[string]string, deps domain.MigrationDeps) (*domain.MigrationReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, data, _, err := vault.Open(r.dir, masterPassword)
	if err != nil {
		return nil, err
	}
	defer releaseScryptPages()

	if !vault.NeedsMigration(data) {
		return nil, fmt.Errorf("vault version %d: %w", data.Version, domain.ErrVaultAlreadyExists)
	}

	fromVersion := data.Version
	if err := vault.BackupVaultFile(r.dir, fromVersion); err != nil {
		return nil, fmt.Errorf("vault migration backup: %w", err)
	}

	report, err := vault.MigrateToCurrent(data, deps.Codec, deps.NewDataKey, answers)
	if err != nil {
		return nil, err
	}
	// A vault old enough to need this migration predates the envelope, so the same rewrite that
	// upgrades the schema is also the one that gives it a vault key. The backup above already
	// captured the original bytes under the version being left behind.
	converted := session.IsLegacy()
	if converted {
		if err := session.Rekey(masterPassword); err != nil {
			return nil, fmt.Errorf("vault migration rekey: %w", err)
		}
	}
	if err := session.Save(data); err != nil {
		return nil, fmt.Errorf("vault migration write: %w", err)
	}
	report.BackupPath = vault.BackupPath(r.dir, fromVersion)

	r.session = session
	r.data = data
	r.ensureVaultDataLocked()
	r.unlocked = true
	r.dirty = false
	r.generation = 0
	// A schema this old also predates the envelope, so this rewrite is the one that gave the vault
	// a key to wrap credentials around - which makes it the moment to offer a first recovery key,
	// exactly as an ordinary unlock of a pre-envelope vault does.
	r.converted = converted
	return report, nil
}

// releaseScryptPages hands the vault KDF's transient ~256 MiB back to the OS, the same workaround
// Unlock and flushGeneration use — see the SetWorkFactor comment in internal/infra/vault/vault.go.
func releaseScryptPages() {
	safego.GoNamed("vault.migrateGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
}
