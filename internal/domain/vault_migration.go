package domain

import "context"

// PendingKey names an identity whose passphrase a schema migration needs before it can rewrite
// the key. Only what a user needs to recognise the key travels here; nothing in it is secret.
type PendingKey struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
	KeyType string `json:"keyType"`
}

// MigrationReport records what a migration did.
//
// Skipped is as important as Converted: a migration that could not finish every key has partly
// succeeded, and reporting a bare success would leave the user believing keys are in a state they
// are not, with no prompt to go back and finish them.
type MigrationReport struct {
	FromVersion int      `json:"fromVersion"`
	ToVersion   int      `json:"toVersion"`
	Converted   []string `json:"converted"`
	Skipped     []string `json:"skipped"`
	BackupPath  string   `json:"backupPath"`
}

// MigrationPlan says whether an upgrade is needed and what it will ask the user for.
//
// Required is a field rather than an empty Keys slice standing for "nothing to do": a vault that
// needs migrating but holds no protected keys also produces an empty list, and the two call for
// opposite screens.
type MigrationPlan struct {
	Required bool         `json:"required"`
	Keys     []PendingKey `json:"keys"`
}

// MigrationDeps carries what a schema upgrade needs beyond the vault file itself.
type MigrationDeps struct {
	Codec      KeyCodec
	NewDataKey func() ([]byte, error)
}

// VaultMigrator upgrades an on-disk vault to the current schema.
//
// Planning and completing are separate so the UI can collect every passphrase in one pass. Asking
// mid-rewrite would leave a half-migrated vault on disk if the user closed the window at the wrong
// moment, and a half-migrated vault is one no build can open.
type VaultMigrator interface {
	PlanMigration(ctx context.Context, masterPassword string, deps MigrationDeps) (*MigrationPlan, error)
	CompleteMigration(ctx context.Context, masterPassword string, answers map[string]string, deps MigrationDeps) (*MigrationReport, error)
}
