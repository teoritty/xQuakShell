package synctrust

import (
	"context"
	"crypto/sha256"

	"xquakshell/internal/domain/vaultsync"
)

const labelPersonalVault = "xqs-sync/v1/personal-vault"

// PersonalVaultID is the id of an account's personal vault. Deriving it from the account means a
// device that logs in finds the personal vault without the server saying which one it is - the
// server could otherwise point it at a vault of its choosing.
func PersonalVaultID(account vaultsync.ID) vaultsync.ID {
	sum := sha256.Sum256(append([]byte(labelPersonalVault), account[:]...))
	var id vaultsync.ID
	copy(id[:], sum[:])
	return id
}

// HelloInfo is what a verified hello established.
type HelloInfo struct {
	IdentityKey []byte
	// Context binds the password exchange to the server's offer (vaultsync.HelloContext).
	Context []byte
}

// Registration is everything an account is created with, accepted or refused together.
type Registration struct {
	Invite        string
	Username      string
	Account       vaultsync.ID
	Record        []byte
	IdentityKey   []byte
	EncryptionKey []byte
	Bundle        []byte
	Device        vaultsync.DeviceEntry
	Vault         vaultsync.MemberOp
}

// LoginResult is what a password login returns: an account token for enrolling this device, and
// the bundle the export key opens.
type LoginResult struct {
	AccountToken  string
	Bundle        []byte
	IdentityKey   []byte
	EncryptionKey []byte
}

// AccountServer is the server as the account flows see it: the sync API plus registration, login
// and device authentication.
type AccountServer interface {
	SyncServer
	EventSource
	Hello(ctx context.Context) (HelloInfo, error)
	SetAccessToken(token string)
	RegisterInit(ctx context.Context, invite, username string, account vaultsync.ID, request []byte) ([]byte, error)
	RegisterFinish(ctx context.Context, r Registration) error
	LoginInit(ctx context.Context, username string, ke1 []byte) (loginID string, account vaultsync.ID, ke2 []byte, err error)
	LoginFinish(ctx context.Context, loginID string, ke3 []byte) (LoginResult, error)
	DeviceChallenge(ctx context.Context, account, device vaultsync.ID) ([]byte, error)
	DeviceVerify(ctx context.Context, account, device vaultsync.ID, nonce, sig []byte) (string, error)
}

// AccountCrypto is the account-key cryptography synccrypto implements.
type AccountCrypto interface {
	GenerateEncryptionKey() (seed, public []byte, err error)
	EncryptionPublic(seed []byte) ([]byte, error)
	SealBundle(exportKey []byte, account vaultsync.ID, identitySeed, encryptionSeed []byte) ([]byte, error)
	OpenBundle(exportKey []byte, account vaultsync.ID, sealed []byte) (identitySeed, encryptionSeed []byte, err error)
	WrapVaultKey(memberPublic []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, vk []byte) ([]byte, error)
	UnwrapVaultKey(seed []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, wrapped []byte) ([]byte, error)
}
