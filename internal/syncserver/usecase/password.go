package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain/vaultsync"
)

// maxPasswordRequest bounds an OPAQUE registration request; suite 1 requests are 32 bytes.
const maxPasswordRequest = 1024

// PasswordStart answers an OPAQUE re-registration for the caller's account (spec §5.5, §5.7). It
// needs no old password: a signed-in device may reset a forgotten one, and what authorises the
// change is the account key's signature checked in PasswordFinish.
func (s *Service) PasswordStart(ctx context.Context, p Principal, request []byte) ([]byte, error) {
	if len(request) == 0 || len(request) > maxPasswordRequest {
		return nil, ErrMalformed
	}
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		_, err := activeAccount(tx, p.Account)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.cfg.AKE.RegistrationResponse(request, p.Account)
}

// PasswordFinish replaces the account's OPAQUE record and bundle. The account's current identity
// key, from the replayed device log, must sign the pair: a session alone - a stolen token, or a
// revoked device holding a superseded key - cannot swap in a bundle of its choosing. Every other
// session of the account ends, following ADR-021 rule 6: a credential change revokes earlier ones.
func (s *Service) PasswordFinish(ctx context.Context, p Principal, record, bundle, sig []byte) error {
	if len(bundle) == 0 || len(bundle) > maxBundleSize {
		return ErrMalformed
	}
	if err := s.cfg.AKE.ValidateRecord(record); err != nil {
		return malformed(err)
	}
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		acct, err := activeAccount(tx, p.Account)
		if err != nil {
			return err
		}
		entries, err := tx.DeviceEntries(acct.ID)
		if err != nil {
			return err
		}
		log, err := replayDeviceLog(acct, entries)
		if err != nil {
			return err
		}
		msg, err := vaultsync.RekeyMessage(acct.ID, record, bundle)
		if err != nil {
			return err
		}
		if vaultsync.VerifySignature(vaultsync.SigEd25519, log.IdentityKey(), msg, sig) != nil {
			return ErrUnauthorized
		}
		if err := tx.UpdateAccountCredentials(acct.ID, record, bundle); err != nil {
			return err
		}
		return tx.DeleteTokensForAccountExcept(acct.ID, p.Device)
	})
}

// activeAccount loads the caller's account, reporting a frozen one as such.
func activeAccount(tx Tx, id vaultsync.ID) (Account, error) {
	acct, ok, err := tx.AccountByID(id)
	if err != nil || !ok {
		return Account{}, errors.Join(ErrUnauthorized, err)
	}
	if acct.Frozen {
		return Account{}, ErrFrozen
	}
	return acct, nil
}
