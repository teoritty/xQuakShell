package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// ErrHistoryPruned answers a pull from before history the server deleted: commits there lost record
// versions and no longer verify, so the client must start again from the snapshot (spec §7.5).
var ErrHistoryPruned = errors.New("usecase: history before this seq was pruned")

const dayMillis uint64 = 24 * 60 * 60 * 1000

// Versions returns the retained versions of one record, oldest first, for the trash: the newest of
// them up to versionsBytes of ciphertext, which is all the trash reads - it restores the latest
// version it can open. Non-members get ErrNotFound, as on every vault endpoint.
func (s *Service) Versions(ctx context.Context, p Principal, vault, record vaultsync.ID) ([]vaultsync.Envelope, error) {
	if err := requireAccess(p); err != nil {
		return nil, err
	}
	var out []vaultsync.Envelope
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		var err error
		out, err = tx.RecordVersions(vault, record, versionsBytes)
		return err
	})
	return out, err
}

// PruneAll is the janitor's pass over every vault: it deletes history that aged out on the server
// clock and what retired epochs leave behind. Commits and membership appends prune their own vault
// too; this pass covers the vaults nobody writes to.
func (s *Service) PruneAll(ctx context.Context) error {
	var vaults []vaultsync.ID
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		var err error
		vaults, err = tx.AllVaults()
		return err
	})
	if err != nil {
		return err
	}
	for _, v := range vaults {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
			m, err := replayMembership(tx, v)
			if err != nil {
				return err
			}
			return s.pruneVault(tx, v, m)
		}); err != nil {
			return fmt.Errorf("prune vault %s: %w", v, err)
		}
	}
	return nil
}

// pruneVault deletes, in the caller's transaction: versions superseded more than a window ago,
// superseded versions under retired epochs, and the ciphertext of wraps under retired epochs (spec
// §10.1). The server applies the same retirement rule the clients do, on its own clock; deleting
// early would only cost the vault's own members their trash, never another vault's data.
func (s *Service) pruneVault(tx Tx, vault vaultsync.ID, m *vaultsync.MembershipState) error {
	now := s.now()
	var newest uint64
	if window := uint64(m.RetentionDays()) * dayMillis; now >= window {
		seq, err := tx.DeleteSuperseded(vault, now-window+1)
		if err != nil {
			return err
		}
		newest = seq
	}
	retired, err := retiredEpochs(tx, vault, m, now)
	if err != nil {
		return err
	}
	seq, err := tx.DeleteEpochVersions(vault, retired)
	if err != nil {
		return err
	}
	if err := stripRetiredWraps(tx, vault, retired); err != nil {
		return err
	}
	if newest = max(newest, seq); newest > 0 {
		if err := tx.RaisePrunedSeq(vault, newest); err != nil {
			return err
		}
	}
	return tx.RecomputeBytes(vault)
}

// retiredEpochs lists the epochs no live current version uses whose time has come. A tombstone
// carries no ciphertext, so it holds no epoch in use.
func retiredEpochs(tx Tx, vault vaultsync.ID, m *vaultsync.MembershipState, now uint64) ([]uint32, error) {
	epochs, err := tx.EpochsInUse(vault)
	if err != nil {
		return nil, err
	}
	inUse := map[uint32]bool{}
	for _, e := range epochs {
		inUse[e] = true
	}
	used := func(e uint32) bool { return inUse[e] }
	return vaultsync.RetiredEpochs(m.Rotations(), used, m.RetentionDays(), now), nil
}

// stripRetiredWraps removes the ciphertext of every wrap of a retired epoch. The op signs only each
// wrap's digest, so the stripped log still verifies on every device.
func stripRetiredWraps(tx Tx, vault vaultsync.ID, retired []uint32) error {
	if len(retired) == 0 {
		return nil
	}
	ops, err := tx.MemberOps(vault)
	if err != nil {
		return err
	}
	var changed []vaultsync.MemberOp
	for _, op := range ops {
		if !slices.Contains(retired, op.Epoch) {
			continue
		}
		for i := range op.Wraps {
			op.Wraps[i] = op.Wraps[i].Strip()
		}
		changed = append(changed, op)
	}
	return tx.ReplaceMemberOps(vault, changed)
}

// applyPurges deletes every version of the records a stored commit purged.
func applyPurges(tx Tx, vault vaultsync.ID, purged []vaultsync.ID) error {
	seq, err := tx.DeleteRecords(vault, purged)
	if err != nil || seq == 0 {
		return err
	}
	return tx.RaisePrunedSeq(vault, seq)
}
