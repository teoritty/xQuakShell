package usecase

import (
	"context"
	"fmt"
	"slices"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// bootstrap starts a device that has verified nothing of a vault from the server's snapshot (spec
// §7.5) instead of replaying history: after an epoch rotation, history is sealed under keys a new
// device never receives. Trust in the head comes from its author's certificate in the account's
// device log and the membership at the head's position; every record must carry its own author's
// valid signature, and together they must reproduce the head's Merkle root.
func (p *pullRun) bootstrap(ctx context.Context) error {
	snap, err := p.e.d.Server.Snapshot(ctx, p.vault)
	if err != nil {
		return err
	}
	if snap.Head == nil {
		if len(snap.Records) != 0 {
			return integrity("snapshot", fmt.Errorf("records without a head"))
		}
		return nil
	}
	state, err := p.verifySnapshot(snap)
	if err != nil {
		return err
	}
	if err := p.land(ctx, vaultsync.Commit{Head: *snap.Head, Records: snap.Records}, state); err != nil {
		return err
	}
	p.report.Commits++
	return nil
}

// verifySnapshot checks a snapshot's head against the author rules and every record against its
// author's signature, and returns the state they reproduce.
func (p *pullRun) verifySnapshot(snap synctrust.RemoteSnapshot) (*vaultsync.VaultState, error) {
	head := *snap.Head
	m, err := p.members.at(head.MembershipSeq)
	if err != nil {
		return nil, err
	}
	if err := p.verifySnapshotHead(&head, m); err != nil {
		return nil, integrity("snapshot head", err)
	}
	for i := range snap.Records {
		_, key, err := p.resolve(snap.Records[i].Author)
		if err != nil {
			return nil, integrity("snapshot record", err)
		}
		if err := snap.Records[i].Verify(key); err != nil {
			return nil, integrity("snapshot record", err)
		}
	}
	state, err := vaultsync.RestoreVaultState(p.vault, &head, snap.Records)
	if err != nil {
		return nil, integrity("snapshot", err)
	}
	return state, nil
}

// resync starts again from the snapshot after the server pruned history this device has not seen
// (spec §7.5). The snapshot may not step back: a lower seq is a rollback and the same seq with
// another head a fork. Records that did not change are left out, so the merge against this
// device's base keeps its unpushed edits exactly as a normal pull would.
func (p *pullRun) resync(ctx context.Context) error {
	snap, err := p.e.d.Server.Snapshot(ctx, p.vault)
	if err != nil {
		return err
	}
	if snap.Head == nil {
		return integrity("snapshot", fmt.Errorf("no head after pruned history"))
	}
	hash, err := snap.Head.Hash()
	if err != nil {
		return integrity("snapshot head", err)
	}
	cur := p.state.Cursor()
	if err := vaultsync.CheckObserved(cur, snap.Head.Seq, hash); err != nil {
		return integrity("snapshot", err)
	}
	if snap.Head.Seq == cur.Seq {
		return nil
	}
	state, err := p.verifySnapshot(snap)
	if err != nil {
		return err
	}
	c, meaning := p.snapshotDelta(snap)
	if err := p.landAs(ctx, c, meaning, state); err != nil {
		return err
	}
	p.report.Commits++
	return nil
}

// snapshotDelta is the snapshot as one commit over this device's base: the records whose version
// moved, and the records the server no longer holds, purged. A record absent from a verified
// snapshot was deleted and purged by an author the head answers for - the Merkle root covers the
// whole state, so a server cannot drop one - and a live one here becomes a deletion in meaning, so
// an unpushed local edit still wins over it as edits do over deletes.
func (p *pullRun) snapshotDelta(snap synctrust.RemoteSnapshot) (stored, meaning vaultsync.Commit) {
	stored = vaultsync.Commit{Head: *snap.Head}
	held := make(map[vaultsync.ID]bool, len(snap.Records))
	for _, r := range snap.Records {
		held[r.Record] = true
		if base, ok := p.base[r.Record]; !ok || base.Version != r.Version {
			stored.Records = append(stored.Records, r)
		}
	}
	meaning = vaultsync.Commit{Head: stored.Head, Records: slices.Clone(stored.Records)}
	for id, env := range p.base {
		if held[id] {
			continue
		}
		stored.Purged = append(stored.Purged, id)
		if !env.Deleted {
			meaning.Records = append(meaning.Records, vaultsync.Envelope{Vault: p.vault, Record: id, Version: env.Version + 1, Deleted: true})
		}
	}
	meaning.Purged = stored.Purged
	return stored, meaning
}

// verifySnapshotHead applies the author rules a commit's head must pass: a certified device of a
// member who may write, not revoked at that membership position, at the vault's current epoch.
func (p *pullRun) verifySnapshotHead(h *vaultsync.Head, m *vaultsync.MembershipState) error {
	if h.Vault != p.vault || h.MembershipHead != m.Hash() {
		return vaultsync.ErrMembershipMismatch
	}
	account, key, err := p.resolve(h.Author)
	if err != nil {
		return err
	}
	if m.IsDeviceRevoked(h.Author) {
		return vaultsync.ErrRevokedDevice
	}
	if !m.Role(account).CanWrite() {
		return vaultsync.ErrNotAuthorized
	}
	if h.CurrentEpoch != m.Epoch() {
		return vaultsync.ErrEpochMismatch
	}
	return h.Verify(key)
}
