package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"math"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// errRoundAgain asks Sync for another round: retain changed the vault through another run, so this
// run's view is stale for whatever retention still has to do.
var errRoundAgain = errors.New("sync: run the round again")

// retain runs after a round pushed everything this device had: it purges tombstones whose trash
// window has passed and rotates the vault key every half window (spec §10.1). Both go through the
// outbox like any commit, so a crash resends the same one. A device that may not write does
// neither; another member's device will.
func (p *pullRun) retain(ctx context.Context) error {
	m, err := p.members.at(uint64(len(p.ops)))
	if err != nil {
		return err
	}
	if !m.Role(p.e.d.Keys.Account()).CanWrite() {
		return nil
	}
	// Records left under an older epoch mean a rotation's re-encryption was interrupted; finishing
	// it here is what makes a crashed rotation or Shred now complete on the next round.
	if len(p.staleRecords(m.Epoch())) > 0 {
		if err := p.e.reencrypt(ctx, p.vault); err != nil {
			return err
		}
		return errRoundAgain
	}
	now := uint64(max(p.e.d.NowUnixMilli(), 0))
	rotations := m.Rotations()
	if due := p.dueTombstones(m.RetentionDays(), rotations[len(rotations)-1], now); len(due) > 0 {
		if err := p.purge(ctx, due); err != nil {
			return err
		}
	}
	last := rotations[len(rotations)-1].StartedUnixMilli
	// The vault always counts as changed since its last rotation: every rotation re-encrypts at least
	// the vault-meta record, so a commit always follows it. The half window is the real cadence.
	if !vaultsync.RotationDue(last, true, m.RetentionDays(), now) {
		return nil
	}
	return p.e.RotateEpoch(ctx, p.vault, nil, false, rand.Reader)
}

// dueTombstones lists, in a stable order, the tombstones whose window passed, and after a shred
// rotation every tombstone dated before it. A tombstone's age counts from the signed time of the
// head that carried it.
func (p *pullRun) dueTombstones(retentionDays uint16, last vaultsync.Rotation, now uint64) []vaultsync.ID {
	var out []vaultsync.ID
	for _, id := range p.tombstones() {
		at := p.landed[id]
		if vaultsync.PurgeDue(at, retentionDays, now) || (last.Shred && at <= last.StartedUnixMilli) {
			out = append(out, id)
		}
	}
	return out
}

func sortIDs(ids []vaultsync.ID) {
	slices.SortFunc(ids, func(a, b vaultsync.ID) int { return bytes.Compare(a[:], b[:]) })
}

// millis converts a signed client time for display. Times come from verified heads and never
// approach the int64 range; one that did is capped rather than wrapped negative.
func millis(t uint64) int64 {
	return int64(min(t, math.MaxInt64)) // #nosec G115 -- capped to MaxInt64 on the line itself
}

// purge commits the removal of tombstones from the vault's tree.
func (p *pullRun) purge(ctx context.Context, ids []vaultsync.ID) error {
	c, err := p.buildCommit(nil, ids)
	if err != nil {
		return err
	}
	return p.send(ctx, c)
}
