package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

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
	now := uint64(max(p.e.d.NowUnixMilli(), 0))
	if due := p.dueTombstones(m.RetentionDays(), now, false); len(due) > 0 {
		if err := p.purge(ctx, due); err != nil {
			return err
		}
	}
	rotations := m.Rotations()
	last := rotations[len(rotations)-1].StartedUnixMilli
	// The vault always counts as changed since its last rotation: every rotation re-encrypts at least
	// the vault-meta record, so a commit always follows it. The half window is the real cadence.
	if !vaultsync.RotationDue(last, true, m.RetentionDays(), now) {
		return nil
	}
	return p.e.RotateEpoch(ctx, p.vault, nil, false, rand.Reader)
}

// dueTombstones lists, in a stable order, the tombstones whose window passed - or every tombstone,
// for Shred now. A tombstone's age counts from the signed time of the head that carried it.
func (p *pullRun) dueTombstones(retentionDays uint16, now uint64, all bool) []vaultsync.ID {
	var out []vaultsync.ID
	for id, env := range p.base {
		if env.Deleted && (all || vaultsync.PurgeDue(p.landed[id], retentionDays, now)) {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b vaultsync.ID) int { return bytes.Compare(a[:], b[:]) })
	return out
}

// purge commits the removal of tombstones from the vault's tree.
func (p *pullRun) purge(ctx context.Context, ids []vaultsync.ID) error {
	c, err := p.buildCommit(nil, ids)
	if err != nil {
		return err
	}
	return p.send(ctx, c)
}
