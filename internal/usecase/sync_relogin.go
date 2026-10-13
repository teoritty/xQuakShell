package usecase

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

// Relogin fetches the account keys another device rotated (ErrKeysRotated) by signing in again with
// the sync password. The bundle must hold exactly the keys the replayed device log now ends at, and
// this device must still be certified in it; a revoked device learns so here and goes no further.
func (s *SyncAccountService) Relogin(ctx context.Context, password []byte) (SyncSession, error) {
	defer clear(password)
	data, err := s.d.Vault.GetData()
	if err != nil {
		return SyncSession{}, err
	}
	if data.Sync == nil || data.Sync.Account == nil {
		return SyncSession{}, ErrSyncNotEnabled
	}
	acct := data.Sync.Account
	srv, err := s.d.NewServer(acct.ServerURL, acct.ServerIdentity)
	if err != nil {
		return SyncSession{}, err
	}
	hello, err := srv.Hello(ctx)
	if err != nil {
		return SyncSession{}, err
	}
	next, adopted, err := s.adoptPending(ctx, srv, acct)
	if err != nil {
		return SyncSession{}, err
	}
	if !adopted {
		if next, err = s.refreshedKeys(ctx, srv, hello, acct, password); err != nil {
			return SyncSession{}, err
		}
	}
	keys, err := NewSyncKeyRing(next, s.d.Crypto)
	if err != nil {
		return SyncSession{}, err
	}
	auth := NewAuthenticatedServer(srv, keys)
	if err := auth.Authenticate(ctx); err != nil {
		if errors.Is(err, synctrust.ErrRemoteUnauthorized) {
			return SyncSession{}, ErrSyncDeviceRevoked
		}
		return SyncSession{}, err
	}
	if err := s.storeAccount(ctx, func(a *domain.SyncAccount) { *a = *next }); err != nil {
		return SyncSession{}, err
	}
	return SyncSession{Server: auth, Keys: keys, Personal: synctrust.PersonalVaultID(keys.Account())}, nil
}

// refreshedKeys logs in, opens the bundle and checks it against the device log, returning the stored
// account with the bundle's keys in place of the old ones.
func (s *SyncAccountService) refreshedKeys(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	acct *domain.SyncAccount, password []byte) (*domain.SyncAccount, error) {
	account, res, exportKey, err := s.passwordLogin(ctx, srv, hello, SyncCredentials{Username: acct.Username, Password: password})
	if err != nil {
		return nil, err
	}
	defer clear(exportKey)
	if !bytes.Equal(account[:], acct.Account) {
		return nil, ErrSyncLogin
	}
	opened, err := s.openBundle(SyncCredentials{ServerURL: acct.ServerURL, Pin: acct.ServerIdentity, Username: acct.Username}, account, res, exportKey)
	if err != nil {
		return nil, err
	}
	next := *acct
	next.IdentitySeed, next.EncryptionSeed, next.EncryptionKey = opened.IdentitySeed, opened.EncryptionSeed, opened.EncryptionKey
	next.Pending, next.RekeyPending = nil, false
	srv.SetAccessToken(res.AccountToken)
	log, err := s.verifiedLog(ctx, srv, &next)
	if errors.Is(err, synctrust.ErrKeysRotated) {
		return nil, integrity("account bundle", errors.New("the bundle holds other keys than the device log ends at"))
	}
	if err != nil {
		return nil, err
	}
	device := [16]byte(acct.Device)
	if log.IsRevoked(device) || !slices.Contains(log.Devices(), device) {
		return nil, ErrSyncDeviceRevoked
	}
	return &next, nil
}

// adoptPending finishes a succession that crashed between the device log accepting it and this
// device promoting its keys: if the log ends at the pending keys, they become the account's, with
// the rekey still to do. The server's bundle is the old one then, so refreshing from it would fail.
func (s *SyncAccountService) adoptPending(ctx context.Context, srv synctrust.AccountServer, acct *domain.SyncAccount) (*domain.SyncAccount, bool, error) {
	if acct.Pending == nil {
		return nil, false, nil
	}
	keys, err := NewSyncKeyRing(acct, s.d.Crypto)
	if err != nil {
		return nil, false, err
	}
	auth := NewAuthenticatedServer(srv, keys)
	if err := auth.Authenticate(ctx); err != nil {
		return nil, false, err
	}
	next := *acct
	next.IdentitySeed, next.EncryptionSeed, next.EncryptionKey = acct.Pending.IdentitySeed, acct.Pending.EncryptionSeed, acct.Pending.EncryptionKey
	next.Pending, next.RekeyPending = nil, true
	if _, err := s.verifiedLog(ctx, auth, &next); err != nil {
		if errors.Is(err, synctrust.ErrKeysRotated) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &next, true, nil
}
