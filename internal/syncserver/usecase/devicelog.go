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
	var seq uint64
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
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
		seq = log.Seq()
		return syncDevices(tx, acct.ID, entries)
	})
	// The account's other devices are told at once: an unexpected new device is how a user learns
	// their sync password is known to someone else (spec section 2.5).
	if err == nil {
		s.events.publish([]vaultsync.ID{p.Account}, Event{Account: p.Account, DeviceLogSeq: seq})
	}
	return err
}

// syncDevices mirrors certifications and revocations into the devices table and drops a revoked
// device's tokens in the same transaction, so there is no moment where it is revoked in the log but
// still authenticated.
//
// A device log is checked against its own account alone, so it cannot tell that a certified id is
// already another account's; the table can, and every write here is held to the account the log
// belongs to. The whole append fails if one entry would touch a device that is not this account's.
func syncDevices(tx Tx, account vaultsync.ID, entries []vaultsync.DeviceEntry) error {
	for _, e := range entries {
		switch e.Op {
		case vaultsync.OpAddDevice:
			if err := insertDevice(tx, Device{ID: e.Cert.Device, Account: account, SigningKey: e.Cert.SigningKey}); err != nil {
				return err
			}
		case vaultsync.OpRevokeDevice:
			revoked, err := tx.RevokeDevice(account, e.Revoked)
			if err != nil {
				return err
			}
			if !revoked {
				return malformed(fmt.Errorf("revocation of %s, which is not this account's device", e.Revoked))
			}
			if err := tx.DeleteTokensForDevice(e.Revoked); err != nil {
				return err
			}
		}
	}
	return nil
}

// insertDevice records a device certification, refusing an id another device already holds. Ids are
// random 128-bit values each device draws for itself, so a taken one is never an accident.
func insertDevice(tx Tx, d Device) error {
	inserted, err := tx.InsertDevice(d)
	if err != nil {
		return err
	}
	if !inserted {
		return fmt.Errorf("%w: device id %s is already certified", ErrForbidden, d.ID)
	}
	return nil
}

// LookupResult is what an inviting admin needs about an account.
// GenesisKeys are an account's genesis public keys, from which its device log replays.
type GenesisKeys struct {
	Suite         vaultsync.SuiteID
	IdentityKey   []byte
	EncryptionKey []byte
}

// AccountKeys returns another account's genesis keys to a caller that shares a vault with it, as
// DeviceLog returns its log: a co-member needs both to verify that account's devices. The caller
// trusts them only if the identity key matches the fingerprint the membership log recorded.
func (s *Service) AccountKeys(ctx context.Context, p Principal, account vaultsync.ID) (GenesisKeys, error) {
	var out GenesisKeys
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if p.Account != account {
			shares, err := tx.SharesVault(p.Account, account)
			if err != nil || !shares {
				return errors.Join(ErrNotFound, err)
			}
		}
		a, ok, err := tx.AccountByID(account)
		if err != nil || !ok {
			return errors.Join(ErrNotFound, err)
		}
		out = GenesisKeys{Suite: vaultsync.Suite1, IdentityKey: a.IdentityKey, EncryptionKey: a.EncryptionKey}
		return nil
	})
	return out, err
}

type LookupResult struct {
	Account       vaultsync.ID
	IdentityKey   []byte
	EncryptionKey []byte
	DeviceLogSeq  uint64
	// GenesisEncryptionKey and Entries let an inviter replay the account's log itself and wrap to
	// the encryption key the log ends with, rather than to EncryptionKey on this server's word.
	GenesisEncryptionKey []byte
	Entries              []vaultsync.DeviceEntry
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
		res = LookupResult{Account: acct.ID, IdentityKey: acct.IdentityKey, EncryptionKey: log.EncryptionKey(), DeviceLogSeq: log.Seq(),
			GenesisEncryptionKey: acct.EncryptionKey, Entries: entries}
		return nil
	})
	return res, err
}
