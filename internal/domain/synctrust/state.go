package synctrust

import (
	"context"
	"errors"

	"xquakshell/internal/domain/vaultsync"
)

// StoredVault is what this device has verified of one vault: the head of the last commit it applied
// and the version of every record that commit left current, tombstones included. The engine rebuilds
// a vaultsync.VaultState from it with RestoreVaultState, which re-checks the Merkle root, so a state
// file edited on disk is caught on the next start rather than trusted.
type StoredVault struct {
	// Head is nil before the device has applied any commit of the vault.
	Head    *vaultsync.Head
	Records []vaultsync.Envelope
}

// QuarantineItem is a record that verified but was not applied: its body failed validation, its
// schema is unknown, Classify refused it, or Classify held it for the master password. It stays
// here, out of the vault, until the user confirms or drops it, or a newer version replaces it.
type QuarantineItem struct {
	Envelope    vaultsync.Envelope
	Reason      string
	AtUnixMilli int64
	// Pending marks a change held for the master password (ActionPending) rather than refused: the
	// user can confirm it, where a refused record can only be dropped.
	Pending bool
}

var (
	// ErrStaleHead is an attempt to move a vault's head backwards or sideways. The engine never
	// does this; the store refusing it too means one engine bug cannot roll a vault back.
	ErrStaleHead = errors.New("sync state: head does not advance")
	// ErrLogGap is a device-log or membership entry that does not follow the last one stored.
	ErrLogGap = errors.New("sync state: log entry out of sequence")
	// ErrOutboxBusy is a second pending commit for a vault that already has one.
	ErrOutboxBusy = errors.New("sync state: a commit is already pending")
)

// StateStore is the device's sync state (spec §5.8, plan P4-3). It holds envelopes and signed logs
// only - ciphertext and public data - so nothing in it opens without the keys in the vault.
type StateStore interface {
	LoadVault(ctx context.Context, vault vaultsync.ID) (StoredVault, error)
	// AdvanceVault stores the records a verified commit changed and moves the head, atomically.
	AdvanceVault(ctx context.Context, head vaultsync.Head, changed []vaultsync.Envelope) error
	ForgetVault(ctx context.Context, vault vaultsync.ID) error

	DeviceLog(ctx context.Context, account vaultsync.ID) ([]vaultsync.DeviceEntry, error)
	AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error
	Membership(ctx context.Context, vault vaultsync.ID) ([]vaultsync.MemberOp, error)
	AppendMembership(ctx context.Context, ops []vaultsync.MemberOp) error

	// PutOutbox records the commit about to be sent, before it is sent. After a crash or a lost
	// reply the same commit - same id - is resent, and the server answers it idempotently.
	PutOutbox(ctx context.Context, c vaultsync.Commit) error
	Outbox(ctx context.Context, vault vaultsync.ID) (*vaultsync.Commit, error)
	ClearOutbox(ctx context.Context, vault, commit vaultsync.ID) error

	Quarantine(ctx context.Context, item QuarantineItem) error
	Quarantined(ctx context.Context, vault vaultsync.ID) ([]QuarantineItem, error)
	Release(ctx context.Context, vault, record vaultsync.ID) error
}
