package usecase

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrSyncLogin is a wrong username or sync password. One error for both, as the server gives:
	// telling them apart would let anyone probe which usernames exist.
	ErrSyncLogin = errors.New("sync: wrong username or sync password")
	// ErrSyncAlreadyEnabled is an attempt to enable sync on a vault that already syncs.
	ErrSyncAlreadyEnabled = errors.New("sync: already enabled on this vault")
)

// personalVaultRetentionDays is the trash window a new personal vault starts with (spec D6).
const personalVaultRetentionDays = 30

// SyncAccountDeps are the ports the account flows drive.
type SyncAccountDeps struct {
	Vault domain.VaultRepository
	// NewServer connects to url, holding the server to pin when one is given.
	NewServer    func(url string, pin []byte) (synctrust.AccountServer, error)
	NewAKE       func(helloContext []byte) (vaultsync.PasswordAKEClient, error)
	Crypto       synctrust.AccountCrypto
	Random       io.Reader
	NowUnixMilli func() int64
	DeviceName   string
	Platform     string
	// NewEngine builds the engine for a session; key rotation drives the personal vault through it.
	NewEngine func(SyncSession) *SyncEngine
}

// SyncAccountService enables sync on this device: it checks the server's identity, registers a new
// account or logs in to an existing one, and enrols this device.
type SyncAccountService struct {
	d SyncAccountDeps
}

// NewSyncAccountService wires the account flows; nothing touches the network until a call.
func NewSyncAccountService(d SyncAccountDeps) *SyncAccountService { return &SyncAccountService{d: d} }

// ServerProbe is the identity a server presented, for the user to compare with the fingerprint the
// server's admin printed before anything else is sent to it.
type ServerProbe struct {
	IdentityKey []byte
	Fingerprint string
}

// SyncSession is an enabled, signed-in device.
type SyncSession struct {
	Server   *AuthenticatedServer
	Keys     synctrust.KeyRing
	Personal vaultsync.ID
}

// Probe fetches a server's identity on first contact (trust on first use, confirmed by a person).
func (s *SyncAccountService) Probe(ctx context.Context, url string) (ServerProbe, error) {
	srv, err := s.d.NewServer(url, nil)
	if err != nil {
		return ServerProbe{}, err
	}
	h, err := srv.Hello(ctx)
	if err != nil {
		return ServerProbe{}, err
	}
	return ServerProbe{IdentityKey: h.IdentityKey, Fingerprint: vaultsync.AccountFingerprint(h.IdentityKey)}, nil
}

// SyncCredentials identify the account and the server the user confirmed.
type SyncCredentials struct {
	ServerURL string
	// Pin is the identity key from Probe, after the user confirmed its fingerprint.
	Pin      []byte
	Username string
	// Password is the sync password; it is cleared before the call returns.
	Password []byte
	// Invite is the server admin's invite code; Register needs it, Login ignores it.
	Invite string
}

func (s *SyncAccountService) connect(ctx context.Context, c SyncCredentials) (synctrust.AccountServer, synctrust.HelloInfo, error) {
	if len(c.Pin) != ed25519.PublicKeySize {
		return nil, synctrust.HelloInfo{}, synctrust.ErrServerIdentity
	}
	if err := s.requireDisabled(); err != nil {
		return nil, synctrust.HelloInfo{}, err
	}
	srv, err := s.d.NewServer(c.ServerURL, c.Pin)
	if err != nil {
		return nil, synctrust.HelloInfo{}, err
	}
	h, err := srv.Hello(ctx)
	return srv, h, err
}

func (s *SyncAccountService) requireDisabled() error {
	data, err := s.d.Vault.GetData()
	if err != nil {
		return err
	}
	if data.Sync != nil && data.Sync.Account != nil {
		return ErrSyncAlreadyEnabled
	}
	return nil
}

// Register creates an account with this device as its first and the personal vault's genesis.
func (s *SyncAccountService) Register(ctx context.Context, c SyncCredentials) (SyncSession, error) {
	defer clear(c.Password)
	srv, hello, err := s.connect(ctx, c)
	if err != nil {
		return SyncSession{}, err
	}
	acct, err := s.newAccount(c)
	if err != nil {
		return SyncSession{}, err
	}
	reg, err := s.registration(ctx, srv, hello, acct, c)
	if err != nil {
		return SyncSession{}, err
	}
	if err := srv.RegisterFinish(ctx, reg); err != nil {
		return SyncSession{}, fmt.Errorf("register: %w", err)
	}
	return s.finish(ctx, srv, acct)
}

// newAccount generates the account and device keys.
func (s *SyncAccountService) newAccount(c SyncCredentials) (*domain.SyncAccount, error) {
	account, err := vaultsync.NewID(s.d.Random)
	if err != nil {
		return nil, err
	}
	encSeed, encKey, err := s.d.Crypto.GenerateEncryptionKey()
	if err != nil {
		return nil, err
	}
	_, idKey, err := ed25519.GenerateKey(s.d.Random)
	if err != nil {
		return nil, err
	}
	acct := &domain.SyncAccount{ServerURL: c.ServerURL, ServerIdentity: bytes.Clone(c.Pin), Username: c.Username,
		Account: account[:], IdentitySeed: idKey.Seed(), EncryptionSeed: encSeed, EncryptionKey: encKey,
		GenesisIdentityKey: idKey.Public().(ed25519.PublicKey), GenesisEncryptionKey: bytes.Clone(encKey)}
	return acct, s.newDevice(acct)
}

func (s *SyncAccountService) newDevice(acct *domain.SyncAccount) error {
	device, err := vaultsync.NewID(s.d.Random)
	if err != nil {
		return err
	}
	_, dk, err := ed25519.GenerateKey(s.d.Random)
	if err != nil {
		return err
	}
	acct.Device, acct.DeviceSeed = device[:], dk.Seed()
	return nil
}

func (s *SyncAccountService) registration(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	acct *domain.SyncAccount, c SyncCredentials) (synctrust.Registration, error) {
	account := vaultsync.ID(acct.Account)
	ake, err := s.d.NewAKE(hello.Context)
	if err != nil {
		return synctrust.Registration{}, err
	}
	request, err := ake.RegistrationStart(c.Password)
	if err != nil {
		return synctrust.Registration{}, err
	}
	response, err := srv.RegisterInit(ctx, c.Invite, c.Username, account, request)
	if err != nil {
		return synctrust.Registration{}, fmt.Errorf("register: %w", err)
	}
	record, exportKey, err := ake.RegistrationFinish(response, account)
	if err != nil {
		return synctrust.Registration{}, err
	}
	defer clear(exportKey)
	bundle, err := s.d.Crypto.SealBundle(exportKey, account, acct.IdentitySeed, acct.EncryptionSeed)
	if err != nil {
		return synctrust.Registration{}, err
	}
	genesisLog, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, identityPublic(acct), acct.EncryptionKey)
	if err != nil {
		return synctrust.Registration{}, err
	}
	entry, err := s.addDeviceEntry(acct, genesisLog)
	if err != nil {
		return synctrust.Registration{}, err
	}
	op, err := s.personalGenesis(acct)
	if err != nil {
		return synctrust.Registration{}, err
	}
	return synctrust.Registration{Invite: c.Invite, Username: c.Username, Account: account, Record: record,
		IdentityKey: identityPublic(acct), EncryptionKey: acct.EncryptionKey, Bundle: bundle, Device: entry, Vault: op}, nil
}

func identityPublic(acct *domain.SyncAccount) []byte {
	return ed25519.NewKeyFromSeed(acct.IdentitySeed).Public().(ed25519.PublicKey)
}

// addDeviceEntry certifies this device in the account's log, signed with the account identity key.
func (s *SyncAccountService) addDeviceEntry(acct *domain.SyncAccount, log *vaultsync.DeviceLogState) (vaultsync.DeviceEntry, error) {
	e := vaultsync.DeviceEntry{Account: vaultsync.ID(acct.Account), Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpAddDevice,
		Cert: &vaultsync.DeviceCert{Device: vaultsync.ID(acct.Device), SigningKey: ed25519.NewKeyFromSeed(acct.DeviceSeed).Public().(ed25519.PublicKey),
			Name: s.d.DeviceName, Platform: s.d.Platform, CreatedUnixMilli: uint64(max(s.d.NowUnixMilli(), 0))},
		SigAlg: vaultsync.SigEd25519}
	msg, err := e.SigningBytes()
	if err != nil {
		return e, err
	}
	e.Sig = ed25519.Sign(ed25519.NewKeyFromSeed(acct.IdentitySeed), msg)
	return e, nil
}

// personalGenesis creates the personal vault with a fresh vault key wrapped to the account.
func (s *SyncAccountService) personalGenesis(acct *domain.SyncAccount) (vaultsync.MemberOp, error) {
	account := vaultsync.ID(acct.Account)
	vault := synctrust.PersonalVaultID(account)
	vk := make([]byte, 32)
	if _, err := io.ReadFull(s.d.Random, vk); err != nil {
		return vaultsync.MemberOp{}, err
	}
	defer clear(vk)
	wrapped, err := s.d.Crypto.WrapVaultKey(acct.EncryptionKey, vault, 0, account, vk)
	if err != nil {
		return vaultsync.MemberOp{}, err
	}
	genesis, err := vaultsync.NewMembershipState(vault)
	if err != nil {
		return vaultsync.MemberOp{}, err
	}
	op := vaultsync.MemberOp{Vault: vault, Seq: 1, Prev: genesis.Hash(), Kind: vaultsync.MemberCreate, Author: account,
		AuthorDevice: vaultsync.ID(acct.Device), Suite: vaultsync.Suite1, RetentionDays: personalVaultRetentionDays,
		Wraps: []vaultsync.KeyWrap{{Member: account, Ciphertext: wrapped}}, TimeUnixMilli: uint64(max(s.d.NowUnixMilli(), 0)),
		SigAlg: vaultsync.SigEd25519}
	msg, err := op.SigningBytes()
	if err != nil {
		return op, err
	}
	op.Sig = ed25519.Sign(ed25519.NewKeyFromSeed(acct.DeviceSeed), msg)
	return op, nil
}

// finish signs the device in and stores the account in the vault. The vault is written last: a
// failure before it leaves this device as it was, with nothing half-enabled.
func (s *SyncAccountService) finish(ctx context.Context, srv synctrust.AccountServer, acct *domain.SyncAccount) (SyncSession, error) {
	keys, err := NewSyncKeyRing(acct, s.d.Crypto)
	if err != nil {
		return SyncSession{}, err
	}
	auth := NewAuthenticatedServer(srv, keys)
	if err := auth.Authenticate(ctx); err != nil {
		return SyncSession{}, err
	}
	err = s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil {
			d.Sync = &domain.VaultSync{}
		}
		if d.Sync.Account != nil {
			return ErrSyncAlreadyEnabled
		}
		d.Sync.Account = acct
		return nil
	})
	if err != nil {
		return SyncSession{}, err
	}
	return SyncSession{Server: auth, Keys: keys, Personal: synctrust.PersonalVaultID(keys.Account())}, nil
}
