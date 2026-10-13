package usecase

import (
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// baseBody opens the verified version of a record this device holds, cached for the run. ok is false
// when the device has no live version of it.
func (p *pullRun) baseBody(id vaultsync.ID) (synctrust.Body, bool) {
	if b, ok := p.bodies[id]; ok {
		return b, true
	}
	env, ok := p.base[id]
	if !ok || env.Deleted {
		return synctrust.Body{}, false
	}
	b, err := p.open(&env)
	if err != nil {
		return synctrust.Body{}, false
	}
	p.bodies[id] = b
	return b, true
}

// editedLocally reports an unpushed local edit: the vault holds something other than the version
// this device last verified. No separate dirty flag exists to drift out of step with the vault.
func (p *pullRun) editedLocally(id vaultsync.ID, local []byte) bool {
	b, ok := p.baseBody(id)
	return !ok || !synctrust.SameData(b.Data, local)
}

// mergeLocal folds an incoming version into this device's unpushed edit of the same item (spec
// §7.4). An edit beats a deletion, so a remote tombstone over a local edit is skipped and the next
// push re-creates the item. A merge that fails or does not validate falls back to the remote
// version unchanged: never apply something no device wrote and the validators refuse.
func (p *pullRun) mergeLocal(out *interpretation, id vaultsync.ID, entry synctrust.Entry, local []byte) (synctrust.Entry, bool) {
	if local == nil || !p.editedLocally(id, local) {
		return entry, true
	}
	if entry.Data == nil {
		return entry, false
	}
	var base []byte
	if b, ok := p.baseBody(id); ok {
		base = b.Data
	}
	merged, conflicts, err := synctrust.MergeData(entry.Type, base, local, entry.Data)
	if err != nil {
		return entry, true
	}
	candidate := synctrust.Entry{Type: entry.Type, Key: entry.Key, Data: merged}
	if synctrust.Validate(candidate) != nil {
		return entry, true
	}
	ces, err := synctrust.ConflictEntries(entry.Type, entry.Key, conflicts, p.e.d.NowUnixMilli())
	if err != nil {
		// Without its conflict record the local credential would be lost; keep the local version
		// and let the next push carry it instead.
		return entry, false
	}
	out.changes = append(out.changes, ces...)
	return candidate, true
}
