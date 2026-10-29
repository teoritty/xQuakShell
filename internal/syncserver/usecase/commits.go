package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain/vaultsync"
)

const (
	maxRecordsPerCommit = 1000
	maxChangesPage      = 200
	// A head is a few hundred bytes of JSON, so a page of them stays far below what a client accepts.
	maxHeadsPage = 1000
	// The page budgets count ciphertext. Base64 and the envelope fields add about a third on the
	// wire, which keeps every page well inside the 64 MiB a client accepts in one reply; a single
	// commit or record is never split and is bounded by the commit body limit instead.
	changesPageBytes    = 16 << 20
	snapshotPageRecords = 1000
	snapshotPageBytes   = 8 << 20
	versionsBytes       = 16 << 20
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
		m, next, err := verifyCommit(tx, vault, c)
		if err != nil {
			return err
		}
		if next.Len() > s.cfg.MaxRecords {
			return fmt.Errorf("%w: %d records, the limit is %d", ErrQuotaExceeded, next.Len(), s.cfg.MaxRecords)
		}
		if err := checkPurgesDue(tx, vault, m, c.Purged, s.now()); err != nil {
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
func verifyCommit(tx Tx, vault vaultsync.ID, c vaultsync.Commit) (*vaultsync.MembershipState, *vaultsync.VaultState, error) {
	m, err := replayMembership(tx, vault)
	if err != nil {
		return nil, nil, err
	}
	state, err := currentState(tx, vault)
	if err != nil {
		return nil, nil, err
	}
	next, err := state.ApplyCommit(c, m, deviceResolver(tx))
	if err != nil {
		return nil, nil, refusal(err)
	}
	return m, next, nil
}

// checkPurgesDue holds every purge to the vault's trash window, counted on this server's clock from
// when it received the tombstone (spec §10.1). Clients decide when to purge from signed client times
// they cannot check against anything, and a purge deletes every version of the record in the same
// transaction - so this server, which does the deleting, is where the window has to hold. A
// tombstone committed before the latest Shred now rotation is due at once: shredding is the vault
// managers' explicit choice to empty the trash, and the membership log orders the two without a
// clock. verifyCommit has already checked each purged id names a current tombstone.
func checkPurgesDue(tx Tx, vault vaultsync.ID, m *vaultsync.MembershipState, purged []vaultsync.ID, now uint64) error {
	window := uint64(m.RetentionDays()) * dayMillis
	for _, id := range purged {
		received, membershipSeq, ok, err := tx.VersionLanded(vault, id)
		if err != nil {
			return err
		}
		if !ok {
			return malformed(fmt.Errorf("purge of %s, which this server does not hold", id))
		}
		if membershipSeq < m.ShredSeq() {
			continue
		}
		if now < received || now-received < window {
			return fmt.Errorf("%w: record %s", ErrPurgeNotDue, id)
		}
	}
	return nil
}

// currentState rebuilds the vault's verified state from the stored leaves. Rebuilding it from the
// envelopes read and hashed every ciphertext the vault held, on every commit - a vault filled to its
// quota made each small commit cost a gigabyte of reading inside the one write transaction.
func currentState(tx Tx, vault vaultsync.ID) (*vaultsync.VaultState, error) {
	head, ok, err := tx.LatestHead(vault)
	if err != nil {
		return nil, err
	}
	leaves, err := tx.CurrentLeaves(vault)
	if err != nil {
		return nil, err
	}
	if !ok {
		return vaultsync.RestoreVaultStateFromLeaves(vault, nil, leaves)
	}
	return vaultsync.RestoreVaultStateFromLeaves(vault, &head, leaves)
}

// Changes returns up to limit whole commits after since, and whether more follow. A page also stops
// once its ciphertext would pass changesPageBytes, so a page of large commits stays within what a
// client accepts in one reply; at least one commit is always returned, so a pull always advances.
func (s *Service) Changes(ctx context.Context, p Principal, vault vaultsync.ID, since uint64, limit int) ([]StoredCommit, bool, error) {
	limit = max(1, min(limit, maxChangesPage))
	var out []StoredCommit
	var more bool
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		v, _, err := requireMember(tx, vault, p.Account)
		if err != nil {
			return err
		}
		if since < v.PrunedSeq {
			return ErrHistoryPruned
		}
		out, more, err = tx.Commits(vault, since, limit, changesPageBytes)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return out, more, nil
}

// Heads returns up to maxHeadsPage heads after since and at most until, in seq order. They outlive
// the record versions pruning deletes, which is what lets a device moved onto the snapshot by
// ErrHistoryPruned check that the snapshot extends the chain it verified: without them, the only
// thing tying the two together would be this server's word (spec §7.5).
func (s *Service) Heads(ctx context.Context, p Principal, vault vaultsync.ID, since, until uint64) ([]vaultsync.Head, error) {
	var out []vaultsync.Head
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		var err error
		out, err = tx.Heads(vault, since, until, maxHeadsPage)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SnapshotPage is one page of a vault's current state: the latest head, and the latest version of
// the records after the page's starting point, in record order.
type SnapshotPage struct {
	Head    *vaultsync.Head
	Records []vaultsync.Envelope
	More    bool
}

// Snapshot returns one page of the latest head and the latest version of every record, for a new
// device; after is the last record id of the previous page, nil for the first. Each page carries the
// head it was read under, so the client can tell when the vault moved between pages and start again:
// the pages of one head are one consistent state.
//
// A vault's whole state used to go out in one reply. A vault near its quota was then read into
// memory whole, inside the transaction every request queues behind, and still could not reach a
// client, which refuses a reply over 64 MiB.
func (s *Service) Snapshot(ctx context.Context, p Principal, vault vaultsync.ID, after *vaultsync.ID) (SnapshotPage, error) {
	var page SnapshotPage
	start := []byte{}
	if after != nil {
		start = after[:]
	}
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		h, ok, err := tx.LatestHead(vault)
		if err != nil {
			return err
		}
		if ok {
			page.Head = &h
		}
		page.Records, page.More, err = tx.CurrentRecordsPage(vault, start, snapshotPageRecords, snapshotPageBytes)
		return err
	})
	if err != nil {
		return SnapshotPage{}, err
	}
	return page, nil
}
