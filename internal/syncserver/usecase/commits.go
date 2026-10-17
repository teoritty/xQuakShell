package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain/vaultsync"
)

const (
	maxRecordsPerCommit = 1000
	maxChangesPage      = 200
)

// Commit verifies and stores one commit on top of expectSeq. A repeated commit id returns the seq the
// first copy got, so a client that lost the reply can retry safely; a stale expectSeq is a conflict
// naming the seq to rebase onto.
func (s *Service) Commit(ctx context.Context, p Principal, vault vaultsync.ID, expectSeq uint64, c vaultsync.Commit) (uint64, error) {
	if err := requireAccess(p); err != nil {
		return 0, err
	}
	if c.Head.Author != p.Device {
		return 0, ErrForbidden
	}
	if len(c.Records)+len(c.Purged) > maxRecordsPerCommit {
		return 0, ErrTooLarge
	}
	var seq uint64
	var members []vaultsync.ID
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		v, _, err := requireMember(tx, vault, p.Account)
		if err != nil {
			return err
		}
		if prior, dup, err := tx.CommitSeqByID(vault, c.Head.Commit); err != nil || dup {
			seq = prior
			return err
		}
		if expectSeq != v.Seq {
			return &ConflictError{CurrentSeq: v.Seq}
		}
		bytes, err := s.checkQuota(v, c.Records)
		if err != nil {
			return err
		}
		m, err := verifyCommit(tx, vault, c)
		if err != nil {
			return err
		}
		members = m.Members()
		seq = v.Seq + 1
		if err := tx.InsertCommit(vault, seq, c, s.now()); err != nil {
			return err
		}
		if err := tx.UpdateVaultHead(vault, seq, bytes); err != nil {
			return err
		}
		if err := applyPurges(tx, vault, c.Purged); err != nil {
			return err
		}
		return s.pruneVault(tx, vault, m)
	})
	if err == nil && members != nil {
		s.events.publish(members, Event{Vault: vault, Seq: seq})
	}
	return seq, err
}

func (s *Service) checkQuota(v Vault, records []vaultsync.Envelope) (uint64, error) {
	total := v.Bytes
	for _, r := range records {
		total += uint64(len(r.WrappedDEK) + len(r.Body))
	}
	limit := s.cfg.TeamQuota
	if v.Personal {
		limit = s.cfg.PersonalQuota
	}
	if total > limit {
		return 0, ErrQuotaExceeded
	}
	return total, nil
}

// verifyCommit runs the shared commit rules against the stored state and the current membership.
// Clients run the same rules; the server runs them so a hostile client cannot plant a commit every
// other member would then have to refuse.
func verifyCommit(tx Tx, vault vaultsync.ID, c vaultsync.Commit) (*vaultsync.MembershipState, error) {
	m, err := replayMembership(tx, vault)
	if err != nil {
		return nil, err
	}
	state, err := currentState(tx, vault)
	if err != nil {
		return nil, err
	}
	if _, err := state.ApplyCommit(c, m, deviceResolver(tx)); err != nil {
		return nil, refusal(err)
	}
	return m, nil
}

func currentState(tx Tx, vault vaultsync.ID) (*vaultsync.VaultState, error) {
	head, ok, err := tx.LatestHead(vault)
	if err != nil {
		return nil, err
	}
	records, err := tx.CurrentRecords(vault)
	if err != nil {
		return nil, err
	}
	if !ok {
		return vaultsync.RestoreVaultState(vault, nil, records)
	}
	return vaultsync.RestoreVaultState(vault, &head, records)
}

// Changes returns up to limit whole commits after since, and whether more follow.
func (s *Service) Changes(ctx context.Context, p Principal, vault vaultsync.ID, since uint64, limit int) ([]StoredCommit, bool, error) {
	limit = max(1, min(limit, maxChangesPage))
	var out []StoredCommit
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		v, _, err := requireMember(tx, vault, p.Account)
		if err != nil {
			return err
		}
		if since < v.PrunedSeq {
			return ErrHistoryPruned
		}
		out, err = tx.Commits(vault, since, limit+1)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// Snapshot returns the latest head and the latest version of every record, for a new device.
func (s *Service) Snapshot(ctx context.Context, p Principal, vault vaultsync.ID) (*vaultsync.Head, []vaultsync.Envelope, error) {
	var head *vaultsync.Head
	var records []vaultsync.Envelope
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		h, ok, err := tx.LatestHead(vault)
		if err != nil {
			return err
		}
		if ok {
			head = &h
		}
		records, err = tx.CurrentRecords(vault)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, nil, err
	}
	return head, records, err
}
