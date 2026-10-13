package usecase

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// Login signs in to an existing account and enrols this device in it. The bundle the sync password
// opens must hold exactly the keys the server and the account's device log advertise; anything
// else means the server substituted keys, and nothing is stored.
func (s *SyncAccountService) Login(ctx context.Context, c SyncCredentials) (SyncSession, error) {
	defer clear(c.Password)
	srv, hello, err := s.connect(ctx, c)
	if err != nil {
		return SyncSession{}, err
	}
	account, res, exportKey, err := s.passwordLogin(ctx, srv, hello, c)
	if err != nil {
		return SyncSession{}, err
	}
	defer clear(exportKey)
	acct, err := s.openBundle(c, account, res, exportKey)
	if err != nil {
		return SyncSession{}, err
	}
	srv.SetAccessToken(res.AccountToken)
	if err := s.enrolDevice(ctx, srv, acct); err != nil {
		return SyncSession{}, err
	}
	return s.finish(ctx, srv, acct)
}

func (s *SyncAccountService) passwordLogin(ctx context.Context, srv synctrust.AccountServer, hello synctrust.HelloInfo,
	c SyncCredentials) (vaultsync.ID, synctrust.LoginResult, []byte, error) {
	ake, err := s.d.NewAKE(hello.Context)
	if err != nil {
		return vaultsync.ID{}, synctrust.LoginResult{}, nil, err
	}
	ke1, err := ake.LoginStart(c.Password)
	if err != nil {
		return vaultsync.ID{}, synctrust.LoginResult{}, nil, err
	}
	loginID, account, ke2, err := srv.LoginInit(ctx, c.Username, ke1)
	if err != nil {
		return vaultsync.ID{}, synctrust.LoginResult{}, nil, loginError(err)
	}
	ke3, _, exportKey, err := ake.LoginFinish(ke2, account)
	if err != nil {
		return vaultsync.ID{}, synctrust.LoginResult{}, nil, loginError(err)
	}
	res, err := srv.LoginFinish(ctx, loginID, ke3)
	if err != nil {
		clear(exportKey)
		return vaultsync.ID{}, synctrust.LoginResult{}, nil, loginError(err)
	}
	return account, res, exportKey, nil
}

// loginError folds every way a password login fails into ErrSyncLogin, as the server does.
func loginError(err error) error {
	if errors.Is(err, vaultsync.ErrAuthFailed) || errors.Is(err, synctrust.ErrRemoteUnauthorized) {
		return ErrSyncLogin
	}
	return err
}

func (s *SyncAccountService) openBundle(c SyncCredentials, account vaultsync.ID, res synctrust.LoginResult, exportKey []byte) (*domain.SyncAccount, error) {
	idSeed, encSeed, err := s.d.Crypto.OpenBundle(exportKey, account, res.Bundle)
	if err != nil {
		return nil, integrity("account bundle", err)
	}
	encKey, err := s.d.Crypto.EncryptionPublic(encSeed)
	if err != nil || len(idSeed) != ed25519.SeedSize {
		return nil, integrity("account bundle", errors.Join(err, fmt.Errorf("bundle key sizes")))
	}
	// The genesis keys come from the server unauthenticated. They are only where the device log
	// replay starts; the log must end at exactly the keys the password-sealed bundle holds, which
	// no forged genesis or substituted key can reach without the account's own signatures.
	acct := &domain.SyncAccount{ServerURL: c.ServerURL, ServerIdentity: bytes.Clone(c.Pin), Username: c.Username,
		Account: account[:], IdentitySeed: idSeed, EncryptionSeed: encSeed, EncryptionKey: encKey,
		GenesisIdentityKey: bytes.Clone(res.IdentityKey), GenesisEncryptionKey: bytes.Clone(res.EncryptionKey)}
	return acct, s.newDevice(acct)
}

// enrolDevice verifies the account's device log from the bundle's identity key and appends this
// device's certificate to it.
func (s *SyncAccountService) enrolDevice(ctx context.Context, srv synctrust.AccountServer, acct *domain.SyncAccount) error {
	account := vaultsync.ID(acct.Account)
	entries, err := srv.DeviceLog(ctx, account, 0)
	if err != nil {
		return err
	}
	log, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, acct.GenesisIdentityKey, acct.GenesisEncryptionKey)
	if err != nil {
		return integrity("device log", err)
	}
	for i := range entries {
		if err := log.Apply(&entries[i]); err != nil {
			return integrity("device log", err)
		}
	}
	if !bytes.Equal(log.IdentityKey(), identityPublic(acct)) || !bytes.Equal(log.EncryptionKey(), acct.EncryptionKey) {
		return integrity("device log", fmt.Errorf("the log ends at other keys than the bundle holds"))
	}
	entry, err := s.addDeviceEntry(acct, log)
	if err != nil {
		return err
	}
	return srv.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry})
}
