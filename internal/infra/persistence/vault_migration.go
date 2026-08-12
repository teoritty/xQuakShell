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

// MigrationDeps carries what a schema upgrade needs beyond the vault file itself.
type MigrationDeps struct {
	Codec      domain.KeyCodec
	NewDataKey func() ([]byte, error)
}

// PlanMigration decrypts the vault and reports which keys need a passphrase before it can be
// upgraded, without writing anything.
//
// It is a separate call from CompleteMigration so the wizard can collect every passphrase in one
// pass. The alternative — asking mid-rewrite — would leave a half-migrated vault on disk if the
// user closed the window at the wrong moment.
func (r *VaultRepo) PlanMigration(_ context.Context, masterPassword string, deps MigrationDeps) ([]vault.PendingKey, error) {
	r.mu.RLock()
	dir := r.dir
	r.mu.RUnlock()

	data, err := vault.ReadVaultFile(dir, masterPassword)
	if err != nil {
		return nil, err
	}
	defer releaseScryptPages()

	if !vault.NeedsMigration(data) {
		return nil, nil
	}
	return vault.PlanMigration(data, deps.Codec), nil
}

// CompleteMigration upgrades the vault on disk and leaves the repository unlocked on success.
//
// The backup is taken before anything is written and a failure to take it aborts the whole
// upgrade: a migration that rewrites the only copy of someone's keys with no way back is the one
// outcome this must never produce. BackupVaultFile keeps any earlier backup, so a second attempt
// after a crash cannot replace the original with already-migrated bytes.
//
// answers maps identity ID to passphrase; an ID the user skipped is simply absent.
func (r *VaultRepo) CompleteMigration(_ context.Context, masterPassword string, answers map[string]string, deps MigrationDeps) (*vault.MigrationReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := vault.ReadVaultFile(r.dir, masterPassword)
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
	if err := vault.WriteVaultFile(r.dir, masterPassword, data); err != nil {
		return nil, fmt.Errorf("vault migration write: %w", err)
	}
	report.BackupPath = vault.BackupPath(r.dir, fromVersion)

	r.passphrase = masterPassword
	r.data = data
	r.ensureVaultDataLocked()
	r.unlocked = true
	r.dirty = false
	r.generation = 0
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
