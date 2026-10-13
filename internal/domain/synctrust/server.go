package synctrust

import (
	"context"

	"xquakshell/internal/domain/vaultsync"
)

// RemoteVault is one vault the account belongs to, as far as the server says it has advanced.
// It is a hint for what to pull, never trusted: every commit it leads to is verified.
type RemoteVault struct {
	Vault         vaultsync.ID
	Seq           uint64
	MembershipSeq uint64
}

// RemoteCommit is a commit with the seq the server accepted it at.
type RemoteCommit struct {
	Seq    uint64
	Commit vaultsync.Commit
}

// RemoteSnapshot is the latest version of every record and the head that commits to them; Head is
// nil for a vault without commits.
type RemoteSnapshot struct {
	Head    *vaultsync.Head
	Records []vaultsync.Envelope
}

// SyncServer is the server as the sync engine sees it after a verified hello and with a token set.
// Everything it returns is untrusted until vaultsync verified it.
type SyncServer interface {
	Vaults(ctx context.Context) ([]RemoteVault, error)
	CreateVault(ctx context.Context, genesis vaultsync.MemberOp) error
	Changes(ctx context.Context, vault vaultsync.ID, since uint64, limit int) ([]RemoteCommit, bool, error)
	Snapshot(ctx context.Context, vault vaultsync.ID) (RemoteSnapshot, error)
	// Versions lists the retained versions of one record, oldest first, for the trash.
	Versions(ctx context.Context, vault, record vaultsync.ID) ([]vaultsync.Envelope, error)
	// Commit pushes c on top of c.Head.Seq-1 and returns the seq it was stored at; a vault that moved
	// on answers with *ConflictError.
	Commit(ctx context.Context, c vaultsync.Commit) (uint64, error)
	DeviceLog(ctx context.Context, account vaultsync.ID, since uint64) ([]vaultsync.DeviceEntry, error)
	AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error
	Membership(ctx context.Context, vault vaultsync.ID, since uint64) ([]vaultsync.MemberOp, error)
	AppendMembership(ctx context.Context, vault vaultsync.ID, ops []vaultsync.MemberOp) error
}

// RemoteEvent is a notification that a vault advanced or the account's device log grew. It carries
// no data, only a reason to pull, so a forged or replayed event costs at most one verified pull.
type RemoteEvent struct {
	Vault         vaultsync.ID
	Seq           uint64
	MembershipSeq uint64
	DeviceLogSeq  uint64
}

// EventSource delivers server notifications until ctx ends or the connection drops, when the
// channel closes; the scheduler reconnects.
type EventSource interface {
	Events(ctx context.Context) (<-chan RemoteEvent, error)
}
