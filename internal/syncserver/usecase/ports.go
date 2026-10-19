// Package usecase is the sync server's application layer (ADR-023): accounts, OPAQUE logins,
// device authentication, device logs, vaults, membership and the commit feed. It depends only on
// the domain and on the ports declared here; storage, transport and the password protocol are
// adapters plugged in by cmd/xqsyncd.
//
// Every rule a client also enforces is delegated to vaultsync, so the server is a second, not a
// different, implementation of the same checks. The server is not trusted by clients; it enforces
// the rules anyway so a buggy or hostile client cannot poison what other clients download.
package usecase

import (
	"context"

	"xquakshell/internal/domain/vaultsync"
)

// Clock is the only source of time; tests drive it.
type Clock interface {
	NowUnixMilli() uint64
}

// Store runs fn in one transaction: everything it writes is committed together or not at all.
type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
}

// Account is a registered user. OPAQUERecord and Bundle are opaque to the server: the first is what
// OPAQUE needs to answer a login, the second a ciphertext only the account's password opens.
type Account struct {
	ID            vaultsync.ID
	Username      string
	OPAQUERecord  []byte
	Bundle        []byte
	IdentityKey   []byte
	EncryptionKey []byte
	Created       uint64
	Frozen        bool
}

// Device is a certified device, denormalised from the device log for authentication.
type Device struct {
	ID         vaultsync.ID
	Account    vaultsync.ID
	SigningKey []byte
	Revoked    bool
}

// TokenKind separates an account token (after OPAQUE) from an access token (after a device proof).
type TokenKind string

const (
	TokenAccount TokenKind = "account"
	TokenAccess  TokenKind = "access"
)

// Token is stored by the SHA-256 of its value, so a stolen database holds no usable token.
type Token struct {
	Hash    []byte
	Account vaultsync.ID
	Device  vaultsync.ID
	Kind    TokenKind
	Expires uint64
}

// Vault is the server's bookkeeping for one vault; its content is the feed and the logs.
type Vault struct {
	ID            vaultsync.ID
	Owner         vaultsync.ID
	Seq           uint64
	MembershipSeq uint64
	Bytes         uint64
	Personal      bool
	Deleted       bool
	// PrunedSeq is the newest commit that lost a record version to pruning; a pull from before it
	// cannot be verified and is answered with ErrHistoryPruned.
	PrunedSeq uint64
}

// StoredCommit is a commit with the seq it was accepted at.
type StoredCommit struct {
	Seq    uint64
	Commit vaultsync.Commit
}

// InviteInfo is one invite as the operator sees it. ID is the hex of the stored hash, never a code.
type InviteInfo struct {
	ID      string
	Expires uint64
	Used    bool
}

// ServerStats counts what the server holds. Bytes is ciphertext, the figure quotas are kept against.
type ServerStats struct {
	Accounts int
	Frozen   int
	Vaults   int
	Commits  int
	Bytes    uint64
}

// Tx is the set of queries the use cases run, one method per query.
type Tx interface {
	InsertAccount(a Account) error
	AccountByUsername(username string) (Account, bool, error)
	AccountByID(id vaultsync.ID) (Account, bool, error)
	SetFrozen(username string, frozen bool) (bool, error)
	DeleteAccount(username string) (bool, error)
	ListAccounts() ([]Account, error)

	InsertInvite(codeHash []byte, expires uint64) error
	InviteUsable(codeHash []byte, now uint64) (bool, error)
	ConsumeInvite(codeHash []byte, now uint64, account vaultsync.ID) (bool, error)
	// ListInvites lists every invite by the hex of its stored hash; the code itself is not stored.
	ListInvites() ([]InviteInfo, error)
	// DeleteUnusedInvite deletes the unused invites whose hash hex starts with prefix, and reports
	// how many it deleted.
	DeleteUnusedInvite(prefix string) (int, error)
	// Stats counts what the server holds, for the operator.
	Stats() (ServerStats, error)
	// MemberCount counts a vault's current members.
	MemberCount(vault vaultsync.ID) (int, error)
	// DeleteVault deletes a vault with its commits, record versions and membership.
	DeleteVault(vault vaultsync.ID) error

	DeviceEntries(account vaultsync.ID) ([]vaultsync.DeviceEntry, error)
	AppendDeviceEntries(account vaultsync.ID, entries []vaultsync.DeviceEntry) error
	// InsertDevice records a newly certified device and reports false, changing nothing, when the
	// id is already taken - by this account or any other. Device ids are global: a commit head and a
	// membership op name only the device, and the server resolves the account from it.
	InsertDevice(d Device) (bool, error)
	// RevokeDevice marks one of account's devices revoked and reports false when account holds no
	// device with that id.
	RevokeDevice(account, device vaultsync.ID) (bool, error)
	DeviceByID(id vaultsync.ID) (Device, bool, error)

	InsertToken(t Token) error
	TokenByHash(hash []byte) (Token, bool, error)
	DeleteTokensForDevice(device vaultsync.ID) error
	// DeleteTokensForAccountExcept ends every session of the account but the given device's.
	DeleteTokensForAccountExcept(account, keep vaultsync.ID) error
	UpdateAccountCredentials(account vaultsync.ID, record, bundle []byte) error
	DeleteExpiredTokens(now uint64) error

	InsertVault(v Vault) error
	VaultByID(id vaultsync.ID) (Vault, bool, error)
	UpdateVaultHead(id vaultsync.ID, seq, bytes uint64) error
	SetVaultMembershipSeq(id vaultsync.ID, seq uint64) error
	MemberOps(vault vaultsync.ID) ([]vaultsync.MemberOp, error)
	AppendMemberOps(vault vaultsync.ID, ops []vaultsync.MemberOp) error
	ReplaceMembers(vault vaultsync.ID, roles map[vaultsync.ID]vaultsync.Role) error
	MemberRole(vault, account vaultsync.ID) (vaultsync.Role, error)
	VaultsOf(account vaultsync.ID) ([]Vault, error)
	SharesVault(a, b vaultsync.ID) (bool, error)

	CommitSeqByID(vault, commitID vaultsync.ID) (uint64, bool, error)
	InsertCommit(vault vaultsync.ID, seq uint64, c vaultsync.Commit, received uint64) error
	// Commits returns up to limit commits after since, stopping once their ciphertext would pass
	// maxBytes but never before the first; more reports commits after them.
	Commits(vault vaultsync.ID, since uint64, limit, maxBytes int) (commits []StoredCommit, more bool, err error)
	// CurrentRecordsPage returns the latest versions of the records after after, in record order,
	// bounded like Commits by a count and a byte budget.
	CurrentRecordsPage(vault vaultsync.ID, after []byte, limit, maxBytes int) (records []vaultsync.Envelope, more bool, err error)
	// CurrentLeaves is the Merkle leaf of every record's latest version, read without the envelopes.
	CurrentLeaves(vault vaultsync.ID) ([]vaultsync.Leaf, error)
	// EpochsInUse lists the epochs that still seal a live current version.
	EpochsInUse(vault vaultsync.ID) ([]uint32, error)
	LatestHead(vault vaultsync.ID) (vaultsync.Head, bool, error)
	// RecordVersions returns a record's newest retained versions, oldest first, up to maxBytes of
	// ciphertext and always at least the newest.
	RecordVersions(vault, record vaultsync.ID, maxBytes int) ([]vaultsync.Envelope, error)
	// VersionLanded reports when this server received the commit that carried a record's latest
	// version, and the membership position that commit's head was made under.
	VersionLanded(vault, record vaultsync.ID) (received, membershipSeq uint64, ok bool, err error)

	// Pruning. Each delete returns the newest seq whose commit lost a version, or 0.
	AllVaults() ([]vaultsync.ID, error)
	DeleteRecords(vault vaultsync.ID, records []vaultsync.ID) (uint64, error)
	// DeleteSuperseded deletes versions whose successor's commit arrived before cutoff.
	DeleteSuperseded(vault vaultsync.ID, cutoff uint64) (uint64, error)
	// DeleteEpochVersions deletes superseded versions sealed under the given epochs.
	DeleteEpochVersions(vault vaultsync.ID, epochs []uint32) (uint64, error)
	ReplaceMemberOps(vault vaultsync.ID, ops []vaultsync.MemberOp) error
	RaisePrunedSeq(vault vaultsync.ID, seq uint64) error
	RecomputeBytes(vault vaultsync.ID) error
}
