package usecase

import (
	"context"
	"errors"
	"regexp"
)

// ErrInviteNotFound is a revoke whose id names no unused invite, or more than one.
var ErrInviteNotFound = errors.New("no single unused invite has that id")

// inviteIDLength is how much of the hash hex the operator sees: enough to tell invites apart, and
// a prefix of a hash, so a listed id cannot be turned back into a code that registers.
const inviteIDLength = 12

var inviteID = regexp.MustCompile(`^[0-9a-f]{12}$`)

// ListInvites lists invites for the operator: an id, the expiry and whether one was used.
func (s *Service) ListInvites(ctx context.Context) ([]InviteInfo, error) {
	var out []InviteInfo
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		list, err := tx.ListInvites()
		for _, inv := range list {
			inv.ID = inv.ID[:min(inviteIDLength, len(inv.ID))]
			out = append(out, inv)
		}
		return err
	})
	return out, err
}

// RevokeInvite deletes the unused invite a listed id names. A used invite is the record of an
// account's registration and stays.
func (s *Service) RevokeInvite(ctx context.Context, id string) error {
	if !inviteID.MatchString(id) {
		return ErrInviteNotFound
	}
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		n, err := tx.DeleteUnusedInvite(id)
		if err != nil {
			return err
		}
		if n != 1 {
			// More than one is a prefix collision; the transaction rolls the deletes back.
			return ErrInviteNotFound
		}
		return nil
	})
}

// Stats counts accounts, vaults, commits and stored ciphertext.
func (s *Service) Stats(ctx context.Context) (ServerStats, error) {
	var st ServerStats
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) (err error) {
		st, err = tx.Stats()
		return err
	})
	return st, err
}
