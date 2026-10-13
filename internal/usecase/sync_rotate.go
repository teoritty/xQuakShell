package usecase

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrSyncNotEnabled is an account operation on a vault that does not sync.
	ErrSyncNotEnabled = errors.New("sync: not enabled on this vault")
	// ErrSyncRevokeTarget is a revocation of this device itself or of a device the account does
	// not hold certified.
	ErrSyncRevokeTarget = errors.New("sync: that device cannot be revoked from here")
	// ErrSyncDeviceRevoked is this device signing in again after another device revoked it.
	ErrSyncDeviceRevoked = errors.New("sync: this device was revoked from the account")
)

// rotation is one account key succession (spec §5.4-5.7).
type rotation struct {
	// password, when set, must be the current sync password; a reset from a signed-in device
	// leaves it empty (§5.7).
	password    []byte
	newPassword []byte
	revoke      []vaultsync.ID
}

// ChangePassword re-registers the sync password and rotates the account keys, as every credential
// change must (ADR-021 rule 6). The other devices sign in again with the new password.
func (s *SyncAccountService) ChangePassword(ctx context.Context, old, next []byte) (SyncSession, error) {
	return s.rotate(ctx, rotation{password: old, newPassword: next})
}

// RevokeDevices removes devices from the account in one succession and stops the personal vault
// accepting them. It asks for the sync password: the new bundle is sealed under it.
func (s *SyncAccountService) RevokeDevices(ctx context.Context, devices []vaultsync.ID, password []byte) (SyncSession, error) {
	return s.rotate(ctx, rotation{password: password, newPassword: bytes.Clone(password), revoke: devices})
}

// ResetPassword sets a new sync password from a device that is still signed in, without the old
// one (§5.7): the device's key and the account key it holds authorise it.
func (s *SyncAccountService) ResetPassword(ctx context.Context, next []byte) (SyncSession, error) {
	return s.rotate(ctx, rotation{newPassword: next})
}

func (s *SyncAccountService) rotate(ctx context.Context, r rotation) (SyncSession, error) {
	defer clear(r.password)
	defer clear(r.newPassword)
	acct, keys, srv, hello, err := s.signedIn(ctx)
	if err != nil {
		return SyncSession{}, err
	}
	if r.password != nil {
		if err := s.checkPassword(ctx, srv, hello, acct, r.password); err != nil {
			return SyncSession{}, err
		}
	}
	session := SyncSession{Server: NewAuthenticatedServer(srv, keys), Keys: keys, Personal: synctrust.PersonalVaultID(keys.Account())}
	if err := session.Server.Authenticate(ctx); err != nil {
		return SyncSession{}, err
	}
	log, err := s.verifiedLog(ctx, session.Server, acct)
	if err != nil {
		return SyncSession{}, err
	}
	if err := checkRevocable(log, keys.Device(), r.revoke); err != nil {
		return SyncSession{}, err
	}
	next, err := s.succeed(ctx, session, acct, log, r.revoke)
	if err != nil {
		return SyncSession{}, err
	}
	return s.finishRotation(ctx, srv, hello, next, r)
}

// signedIn connects to the server of the account stored in the vault.
func (s *SyncAccountService) signedIn(ctx context.Context) (*domain.SyncAccount, synctrust.KeyRing, synctrust.AccountServer, synctrust.HelloInfo, error) {
	data, err := s.d.Vault.GetData()
	if err != nil {
		return nil, nil, nil, synctrust.HelloInfo{}, err
	}
	if data.Sync == nil || data.Sync.Account == nil {
		return nil, nil, nil, synctrust.HelloInfo{}, ErrSyncNotEnabled
	}
	acct := data.Sync.Account
	keys, err := NewSyncKeyRing(acct, s.d.Crypto)
	if err != nil {
		return nil, nil, nil, synctrust.HelloInfo{}, err
	}
	srv, err := s.d.NewServer(acct.ServerURL, acct.ServerIdentity)
	if err != nil {
		return nil, nil, nil, synctrust.HelloInfo{}, err
	}
	hello, err := srv.Hello(ctx)
	return acct, keys, srv, hello, err
}

// checkPassword proves the current sync password with a full login, so a mistyped one cannot
// become the new password silently.
func (s *SyncAccountService) checkPassword(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	acct *domain.SyncAccount, password []byte) error {
	account, _, exportKey, err := s.passwordLogin(ctx, srv, hello, SyncCredentials{Username: acct.Username, Password: password})
	clear(exportKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(account[:], acct.Account) {
		return ErrSyncLogin
	}
	return nil
}

// verifiedLog replays the account's device log from its genesis keys and requires it to end at the
// keys this device holds.
func (s *SyncAccountService) verifiedLog(ctx context.Context, srv synctrust.SyncServer, acct *domain.SyncAccount) (*vaultsync.DeviceLogState, error) {
	account := vaultsync.ID(acct.Account)
	entries, err := srv.DeviceLog(ctx, account, 0)
	if err != nil {
		return nil, err
	}
	log, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, acct.GenesisIdentityKey, acct.GenesisEncryptionKey)
	if err != nil {
		return nil, integrity("device log", err)
	}
	for i := range entries {
		if err := log.Apply(&entries[i]); err != nil {
			return nil, integrity("device log", err)
		}
	}
	if !bytes.Equal(log.IdentityKey(), identityPublic(acct)) || !bytes.Equal(log.EncryptionKey(), acct.EncryptionKey) {
		return nil, synctrust.ErrKeysRotated
	}
	return log, nil
}

func checkRevocable(log *vaultsync.DeviceLogState, self vaultsync.ID, revoke []vaultsync.ID) error {
	for _, id := range revoke {
		if id == self || log.IsRevoked(id) || !slices.Contains(log.Devices(), id) {
			return fmt.Errorf("%w: %s", ErrSyncRevokeTarget, id)
		}
	}
	return nil
}

// succeed generates the next account keys, lets the personal vault's current key open under them,
// records them as pending, appends the revocations and the succession, and promotes them.
func (s *SyncAccountService) succeed(ctx context.Context, session SyncSession, acct *domain.SyncAccount,
	log *vaultsync.DeviceLogState, revoke []vaultsync.ID) (*domain.SyncAccount, error) {
	_, newAIK, err := ed25519.GenerateKey(s.d.Random)
	if err != nil {
		return nil, err
	}
	encSeed, encKey, err := s.d.Crypto.GenerateEncryptionKey()
	if err != nil {
		return nil, err
	}
	if err := s.d.NewEngine(session).RewrapCurrent(ctx, session.Personal, encKey); err != nil {
		return nil, fmt.Errorf("rewrap personal vault: %w", err)
	}
	pending := &domain.SyncPendingKeys{IdentitySeed: newAIK.Seed(), EncryptionSeed: encSeed, EncryptionKey: encKey}
	if err := s.storeAccount(ctx, func(a *domain.SyncAccount) { a.Pending = pending }); err != nil {
		return nil, err
	}
	entries, err := successionEntries(acct, log, revoke, newAIK, encKey)
	if err != nil {
		return nil, err
	}
	if err := session.Server.AppendDeviceLog(ctx, entries); err != nil {
		return nil, fmt.Errorf("append succession: %w", err)
	}
	next := *acct
	next.IdentitySeed, next.EncryptionSeed, next.EncryptionKey = pending.IdentitySeed, pending.EncryptionSeed, pending.EncryptionKey
	next.Pending, next.RekeyPending = nil, true
	return &next, s.storeAccount(ctx, func(a *domain.SyncAccount) { *a = next })
}

// successionEntries signs the revocations and the succession, applying each to the replayed log as
// it goes: that yields the next entry's position and refuses a batch the server would refuse.
func successionEntries(acct *domain.SyncAccount, log *vaultsync.DeviceLogState, revoke []vaultsync.ID,
	newAIK ed25519.PrivateKey, newEnc []byte) ([]vaultsync.DeviceEntry, error) {
	account := vaultsync.ID(acct.Account)
	oldAIK := ed25519.NewKeyFromSeed(acct.IdentitySeed)
	var out []vaultsync.DeviceEntry
	for _, id := range append(slices.Clone(revoke), vaultsync.ID{}) {
		e := vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpRevokeDevice,
			Revoked: id, SigAlg: vaultsync.SigEd25519}
		if id.IsZero() {
			e = vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpSuccession,
				NewSuite: vaultsync.Suite1, NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: newEnc,
				SigAlg: vaultsync.SigEd25519}
		}
		msg, err := e.SigningBytes()
		if err != nil {
			return nil, err
		}
		e.Sig = ed25519.Sign(oldAIK, msg)
		if e.Op == vaultsync.OpSuccession {
			e.SuccessorSig = ed25519.Sign(newAIK, msg)
		}
		if err := log.Apply(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// finishRotation seals the new bundle under the new password and moves the personal vault to an
// epoch the revoked devices never held.
func (s *SyncAccountService) finishRotation(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	next *domain.SyncAccount, r rotation) (SyncSession, error) {
	keys, err := NewSyncKeyRing(next, s.d.Crypto)
	if err != nil {
		return SyncSession{}, err
	}
	session := SyncSession{Server: NewAuthenticatedServer(srv, keys), Keys: keys, Personal: synctrust.PersonalVaultID(keys.Account())}
	if err := s.rekey(ctx, srv, hello, next, r.newPassword); err != nil {
		return session, err
	}
	if err := s.storeAccount(ctx, func(a *domain.SyncAccount) { a.RekeyPending = false }); err != nil {
		return session, err
	}
	return session, s.d.NewEngine(session).RotateEpoch(ctx, session.Personal, r.revoke, s.d.Random)
}

// rekey re-registers OPAQUE under password and replaces the bundle, consented by the new account key.
func (s *SyncAccountService) rekey(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	acct *domain.SyncAccount, password []byte) error {
	account := vaultsync.ID(acct.Account)
	ake, err := s.d.NewAKE(hello.Context)
	if err != nil {
		return err
	}
	request, err := ake.RegistrationStart(password)
	if err != nil {
		return err
	}
	response, err := srv.PasswordInit(ctx, request)
	if err != nil {
		return fmt.Errorf("password init: %w", err)
	}
	record, exportKey, err := ake.RegistrationFinish(response, account)
	if err != nil {
		return err
	}
	defer clear(exportKey)
	bundle, err := s.d.Crypto.SealBundle(exportKey, account, acct.IdentitySeed, acct.EncryptionSeed)
	if err != nil {
		return err
	}
	msg, err := vaultsync.RekeyMessage(account, record, bundle)
	if err != nil {
		return err
	}
	return srv.PasswordFinish(ctx, record, bundle, ed25519.Sign(ed25519.NewKeyFromSeed(acct.IdentitySeed), msg))
}

func (s *SyncAccountService) storeAccount(ctx context.Context, change func(*domain.SyncAccount)) error {
	return s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil || d.Sync.Account == nil {
			return ErrSyncNotEnabled
		}
		change(d.Sync.Account)
		return nil
	})
}
