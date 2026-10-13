package usecase

import (
	"context"
	"fmt"

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
	head := *snap.Head
	m, err := p.members.at(head.MembershipSeq)
	if err != nil {
		return err
	}
	if err := p.verifySnapshotHead(&head, m); err != nil {
		return integrity("snapshot head", err)
	}
	for i := range snap.Records {
		_, key, err := p.resolve(snap.Records[i].Author)
		if err != nil {
			return integrity("snapshot record", err)
		}
		if err := snap.Records[i].Verify(key); err != nil {
			return integrity("snapshot record", err)
		}
	}
	state, err := vaultsync.RestoreVaultState(p.vault, &head, snap.Records)
	if err != nil {
		return integrity("snapshot", err)
	}
	if err := p.land(ctx, vaultsync.Commit{Head: head, Records: snap.Records}, state); err != nil {
		return err
	}
	p.report.Commits++
	return nil
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
