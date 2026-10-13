package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"xquakshell/internal/domain/vaultsync"
)

const (
	inviteBytes   = 16
	maxBundleSize = 4096
)

var inviteEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func inviteHash(code string) []byte {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return sum[:]
}

// validUsername allows lower-case letters, digits, dot, underscore and hyphen, 3 to 64 bytes. A
// narrow alphabet keeps usernames from carrying look-alike characters into an admin's listing.
func validUsername(name string) bool {
	if len(name) < 3 || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// CreateInvite issues a one-time registration code valid for ttl milliseconds. Only its hash is
// stored, so the database does not hold usable invites.
func (s *Service) CreateInvite(ctx context.Context, ttl uint64) (string, error) {
	raw, err := s.randomBytes(inviteBytes)
	if err != nil {
		return "", err
	}
	code := inviteEncoding.EncodeToString(raw)
	err = s.cfg.Store.WithTx(ctx, func(tx Tx) error { return tx.InsertInvite(inviteHash(code), s.now()+ttl) })
	return code, err
}

// RegisterStart checks the invite and the identifiers, then answers the OPAQUE registration
// request. Nothing is stored: RegisterFinish repeats every check in the transaction that commits.
func (s *Service) RegisterStart(ctx context.Context, invite, username string, account vaultsync.ID, request []byte) ([]byte, error) {
	if !validUsername(username) || account.IsZero() {
		return nil, ErrMalformed
	}
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if ok, err := tx.InviteUsable(inviteHash(invite), s.now()); err != nil || !ok {
			return errors.Join(ErrForbidden, err)
		}
		return checkIdentifiersFree(tx, username, account)
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.cfg.AKE.RegistrationResponse(request, account)
	if err != nil {
		return nil, malformed(err)
	}
	return resp, nil
}

func checkIdentifiersFree(tx Tx, username string, account vaultsync.ID) error {
	if _, taken, err := tx.AccountByUsername(username); err != nil || taken {
		return errors.Join(ErrForbidden, err)
	}
	if _, taken, err := tx.AccountByID(account); err != nil || taken {
		return errors.Join(ErrForbidden, err)
	}
	return nil
}

// Registration is everything RegisterFinish needs, accepted or refused as a whole.
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

// RegisterFinish creates the account, its first device and its personal vault in one transaction.
// The device certificate must be signed by the identity key being registered, and the vault's
// genesis must be signed by that device, so the server never stores a log it could not verify.
func (s *Service) RegisterFinish(ctx context.Context, r Registration) error {
	if !validUsername(r.Username) || len(r.Bundle) == 0 || len(r.Bundle) > maxBundleSize {
		return ErrMalformed
	}
	if err := s.cfg.AKE.ValidateRecord(r.Record); err != nil {
		return malformed(err)
	}
	account := Account{ID: r.Account, Username: r.Username, OPAQUERecord: r.Record, Bundle: r.Bundle,
		IdentityKey: r.IdentityKey, EncryptionKey: r.EncryptionKey, Created: s.now()}
	device, members, err := verifyGenesis(account, r.Device, r.Vault)
	if err != nil {
		return malformed(err)
	}
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if err := checkIdentifiersFree(tx, r.Username, r.Account); err != nil {
			return err
		}
		if _, exists, err := tx.VaultByID(r.Vault.Vault); err != nil || exists {
			return errors.Join(ErrForbidden, err)
		}
		if ok, err := tx.ConsumeInvite(inviteHash(r.Invite), s.now(), r.Account); err != nil || !ok {
			return errors.Join(ErrForbidden, err)
		}
		return storeRegistration(tx, account, device, r, members)
	})
}

func storeRegistration(tx Tx, a Account, d Device, r Registration, members map[vaultsync.ID]vaultsync.Role) error {
	if err := tx.InsertAccount(a); err != nil {
		return err
	}
	if err := tx.AppendDeviceEntries(a.ID, []vaultsync.DeviceEntry{r.Device}); err != nil {
		return err
	}
	if err := tx.UpsertDevice(d); err != nil {
		return err
	}
	if err := tx.InsertVault(Vault{ID: r.Vault.Vault, Owner: a.ID, MembershipSeq: 1, Personal: true}); err != nil {
		return err
	}
	if err := tx.AppendMemberOps(r.Vault.Vault, []vaultsync.MemberOp{r.Vault}); err != nil {
		return err
	}
	return tx.ReplaceMembers(r.Vault.Vault, members)
}

// verifyGenesis replays the first device log entry and the personal vault's create operation.
func verifyGenesis(a Account, entry vaultsync.DeviceEntry, create vaultsync.MemberOp) (Device, map[vaultsync.ID]vaultsync.Role, error) {
	if entry.Op != vaultsync.OpAddDevice || entry.Seq != 1 || entry.Cert == nil {
		return Device{}, nil, fmt.Errorf("first device log entry must certify a device")
	}
	log, err := replayDeviceLog(a, []vaultsync.DeviceEntry{entry})
	if err != nil {
		return Device{}, nil, err
	}
	device := Device{ID: entry.Cert.Device, Account: a.ID, SigningKey: entry.Cert.SigningKey}
	if create.Kind != vaultsync.MemberCreate || create.Author != a.ID || create.AuthorDevice != device.ID {
		return Device{}, nil, fmt.Errorf("personal vault must be created by the registering device")
	}
	m, err := vaultsync.NewMembershipState(create.Vault)
	if err != nil {
		return Device{}, nil, err
	}
	check := func(account, dev vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
		key, err := log.DeviceKey(dev)
		if err != nil || account != a.ID {
			return vaultsync.ErrBadSignature
		}
		return vaultsync.VerifySignature(alg, key, msg, sig)
	}
	if err := m.Apply(&create, check); err != nil {
		return Device{}, nil, err
	}
	return device, map[vaultsync.ID]vaultsync.Role{a.ID: vaultsync.RoleOwner}, nil
}

// SetFrozen freezes or unfreezes an account by username (admin CLI).
func (s *Service) SetFrozen(ctx context.Context, username string, frozen bool) error {
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		changed, err := tx.SetFrozen(username, frozen)
		if err == nil && !changed {
			return ErrNotFound
		}
		return err
	})
}

// DeleteAccount removes an account, its devices and its tokens (admin CLI).
func (s *Service) DeleteAccount(ctx context.Context, username string) error {
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		deleted, err := tx.DeleteAccount(username)
		if err == nil && !deleted {
			return ErrNotFound
		}
		return err
	})
}

// ListAccounts lists accounts for the admin CLI.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	var out []Account
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		var err error
		out, err = tx.ListAccounts()
		return err
	})
	return out, err
}
