package usecase

import (
	"context"
	"crypto/rand"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrTrashNotFound is a restore of a record that is not deleted, or whose content no longer opens.
	ErrTrashNotFound = errors.New("sync: that item is not in the trash")
	// ErrRestoreWeakens is a restore the trust rules would hold: bringing the old value back would
	// weaken this device, which a restore must not do without the review a held change gets.
	ErrRestoreWeakens = errors.New("sync: restoring that item would weaken this device")
)

// TrashItem is one deleted record as the trash shows it. Type and Key are empty and Restorable is
// false when no retained version opens - purged history, or an epoch that retired.
type TrashItem struct {
	Record           vaultsync.ID
	Type             synctrust.RecordType
	Key              string
	DeletedUnixMilli int64
	PurgeUnixMilli   int64
	Restorable       bool
}

// Trash lists the vault's tombstones with the content of their last readable version.
func (e *SyncEngine) Trash(ctx context.Context, vault vaultsync.ID) ([]TrashItem, error) {
	p, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return nil, err
	}
	retired := p.retiredEpochs(m)
	window := int64(m.RetentionDays()) * 24 * 60 * 60 * 1000
	var out []TrashItem
	for _, id := range p.tombstones() {
		deleted := millis(p.landed[id])
		it := TrashItem{Record: id, DeletedUnixMilli: deleted, PurgeUnixMilli: deleted + window}
		body, ok, err := p.lastReadable(ctx, id, retired)
		if err != nil {
			return nil, err
		}
		if ok {
			it.Type, it.Restorable = body.Type, true
			it.Key, _ = synctrust.NaturalKey(body.Type, body.Data)
		}
		out = append(out, it)
	}
	return out, nil
}

// withMembers pairs a run with its latest membership state.
func withMembers(p *pullRun, err error) (*pullRun, *vaultsync.MembershipState, error) {
	if err != nil {
		return nil, nil, err
	}
	m, err := p.members.at(uint64(len(p.ops)))
	return p, m, err
}

// Restore writes a deleted record's last readable content back into the vault; the next round
// commits it as a new version. A change the trust rules would hold is refused, not applied.
func (e *SyncEngine) Restore(ctx context.Context, vault, record vaultsync.ID) error {
	p, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return err
	}
	if env, ok := p.base[record]; !ok || !env.Deleted {
		return ErrTrashNotFound
	}
	body, ok, err := p.lastReadable(ctx, record, p.retiredEpochs(m))
	if err != nil {
		return err
	}
	if !ok {
		return ErrTrashNotFound
	}
	key, err := synctrust.NaturalKey(body.Type, body.Data)
	if err != nil {
		return ErrTrashNotFound
	}
	entry := synctrust.Entry{Type: body.Type, Key: key, Data: body.Data}
	if err := synctrust.Validate(entry); err != nil {
		return ErrTrashNotFound
	}
	verdict, err := synctrust.Classify(body.Type, nil, body.Data)
	if err != nil || verdict.Action != synctrust.ActionApply {
		return ErrRestoreWeakens
	}
	return e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return synctrust.ApplyScope(d, p.scope, []synctrust.Entry{entry}) })
}

// ShredNow destroys the vault's trash and history (spec §10.1): a shred rotation retires every
// older epoch as soon as the re-encryption lands, and the round after purges every tombstone dated
// before it. Interrupted anywhere, the next round finishes it from what the state shows: records
// left under an older epoch are re-encrypted and tombstones older than the shred are purged.
func (e *SyncEngine) ShredNow(ctx context.Context, vault vaultsync.ID) error {
	if err := e.RotateEpoch(ctx, vault, nil, true, rand.Reader); err != nil {
		return err
	}
	_, err := e.Sync(ctx, vault)
	return err
}

// tombstones lists the deleted records of the verified base in a stable order.
func (p *pullRun) tombstones() []vaultsync.ID {
	var out []vaultsync.ID
	for id, env := range p.base {
		if env.Deleted {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

// retiredEpochs is the set of epochs this device treats as destroyed: no live current version uses
// them and their time has come on this device's clock, whatever the server says.
func (p *pullRun) retiredEpochs(m *vaultsync.MembershipState) map[uint32]bool {
	live := map[uint32]bool{}
	for _, env := range p.base {
		if !env.Deleted {
			live[env.Epoch] = true
		}
	}
	now := uint64(max(p.e.d.NowUnixMilli(), 0))
	out := map[uint32]bool{}
	for _, e := range vaultsync.RetiredEpochs(m.Rotations(), func(e uint32) bool { return live[e] }, m.RetentionDays(), now) {
		out[e] = true
	}
	return out
}

// lastReadable opens the newest retained live version of a record. Everything the server returns
// is checked: the version must belong to this record and vault, precede the tombstone, carry its
// author's valid signature, and not be sealed under a retired epoch - a hostile server serving old
// history must not get it opened.
func (p *pullRun) lastReadable(ctx context.Context, id vaultsync.ID, retired map[uint32]bool) (synctrust.Body, bool, error) {
	versions, err := p.e.d.Server.Versions(ctx, p.vault, id)
	if err != nil {
		return synctrust.Body{}, false, err
	}
	tomb := p.base[id].Version
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		// An older tombstone among the versions fails to open below and is skipped like any other.
		if v.Record != id || v.Vault != p.vault || v.Version >= tomb || retired[v.Epoch] {
			continue
		}
		_, key, err := p.resolve(v.Author)
		if err != nil || v.Verify(key) != nil {
			continue
		}
		if body, err := p.open(&v); err == nil {
			return body, true, nil
		}
	}
	return synctrust.Body{}, false, nil
}
