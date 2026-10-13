package usecase

import (
	"context"
	"errors"
	"fmt"

	"xquakshell/internal/domain/vaultsync"
)

// DeviceLog returns an account's device log after since. An account may read its own log and the
// logs of accounts it shares a vault with - it needs those to verify their devices' signatures -
// and nobody else's; a stranger's log is reported as not found.
func (s *Service) DeviceLog(ctx context.Context, p Principal, account vaultsync.ID, since uint64) ([]vaultsync.DeviceEntry, error) {
	var out []vaultsync.DeviceEntry
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if p.Account != account {
			shares, err := tx.SharesVault(p.Account, account)
			if err != nil || !shares {
				return errors.Join(ErrNotFound, err)
			}
		}
		entries, err := tx.DeviceEntries(account)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Seq > since {
				out = append(out, e)
			}
		}
		return nil
	})
	return out, err
}

// AppendDeviceLog appends entries to the caller's own log after verifying them against the replayed
// log. A batch that ends with a revocation not yet followed by a succession is refused whole: the
// revoked device must never find the old identity key still in force.
func (s *Service) AppendDeviceLog(ctx context.Context, p Principal, entries []vaultsync.DeviceEntry) error {
	if len(entries) == 0 || len(entries) > 16 {
		return ErrMalformed
	}
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		acct, ok, err := tx.AccountByID(p.Account)
		if err != nil || !ok {
			return errors.Join(ErrUnauthorized, err)
		}
		stored, err := tx.DeviceEntries(acct.ID)
		if err != nil {
			return err
		}
		log, err := replayDeviceLog(acct, stored)
		if err != nil {
			return err
		}
		for i := range entries {
			if err := log.Apply(&entries[i]); err != nil {
				return malformed(err)
			}
		}
		if log.PendingSuccession() {
			return malformed(fmt.Errorf("batch ends with a revocation and no succession"))
		}
		if err := tx.AppendDeviceEntries(acct.ID, entries); err != nil {
			return err
		}
		return syncDevices(tx, acct.ID, entries, log)
	})
}

// syncDevices mirrors certifications and revocations into the devices table and drops a revoked
// device's tokens in the same transaction, so there is no moment where it is revoked in the log but
// still authenticated.
func syncDevices(tx Tx, account vaultsync.ID, entries []vaultsync.DeviceEntry, log *vaultsync.DeviceLogState) error {
	for _, e := range entries {
		switch e.Op {
		case vaultsync.OpAddDevice:
			if err := tx.UpsertDevice(Device{ID: e.Cert.Device, Account: account, SigningKey: e.Cert.SigningKey}); err != nil {
				return err
			}
		case vaultsync.OpRevokeDevice:
			key, err := log.DeviceKey(e.Revoked)
			if err != nil {
				return err
			}
			if err := tx.UpsertDevice(Device{ID: e.Revoked, Account: account, SigningKey: key, Revoked: true}); err != nil {
				return err
			}
			if err := tx.DeleteTokensForDevice(e.Revoked); err != nil {
				return err
			}
		}
	}
	return nil
}

// LookupResult is what an inviting admin needs about an account.
type LookupResult struct {
	Account       vaultsync.ID
	IdentityKey   []byte
	EncryptionKey []byte
	DeviceLogSeq  uint64
}

// Lookup returns an account's genesis identity key - the one its fingerprint is computed over - and
// its current encryption key from the replayed log. Frozen accounts are not found.
func (s *Service) Lookup(ctx context.Context, p Principal, username string) (LookupResult, error) {
	var res LookupResult
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		acct, ok, err := tx.AccountByUsername(username)
		if err != nil || !ok || acct.Frozen {
			return errors.Join(ErrNotFound, err)
		}
		entries, err := tx.DeviceEntries(acct.ID)
		if err != nil {
			return err
		}
		log, err := replayDeviceLog(acct, entries)
		if err != nil {
			return err
		}
		res = LookupResult{Account: acct.ID, IdentityKey: acct.IdentityKey, EncryptionKey: log.EncryptionKey(), DeviceLogSeq: log.Seq()}
		return nil
	})
	return res, err
}
