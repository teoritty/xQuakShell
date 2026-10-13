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
}

// StoredCommit is a commit with the seq it was accepted at.
type StoredCommit struct {
	Seq    uint64
	Commit vaultsync.Commit
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
	ConsumeInvite(codeHash []byte, now uint64, account vaultsync.ID) (bool, error)

	DeviceEntries(account vaultsync.ID) ([]vaultsync.DeviceEntry, error)
	AppendDeviceEntries(account vaultsync.ID, entries []vaultsync.DeviceEntry) error
	UpsertDevice(d Device) error
	DeviceByID(id vaultsync.ID) (Device, bool, error)

	InsertToken(t Token) error
	TokenByHash(hash []byte) (Token, bool, error)
	DeleteTokensForDevice(device vaultsync.ID) error
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
	InsertCommit(vault vaultsync.ID, seq uint64, c vaultsync.Commit) error
	Commits(vault vaultsync.ID, since uint64, limit int) ([]StoredCommit, error)
	CurrentRecords(vault vaultsync.ID) ([]vaultsync.Envelope, error)
	LatestHead(vault vaultsync.ID) (vaultsync.Head, bool, error)
}
