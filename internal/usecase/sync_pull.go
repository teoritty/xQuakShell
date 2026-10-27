package usecase

import (
	"context"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// pullRun is the working state of one pull of one vault.
type pullRun struct {
	e         *SyncEngine
	vault     vaultsync.ID
	ops       []vaultsync.MemberOp
	members   *membershipCursor
	resolve   vaultsync.DeviceResolver
	state     *vaultsync.VaultState
	base      map[vaultsync.ID]vaultsync.Envelope
	names     map[vaultsync.ID]syncKeyName
	bodies    map[vaultsync.ID]synctrust.Body
	namingKey []byte
	report    PullReport
	// scope is the vault's place in the local vault: the personal items or one team's.
	scope synctrust.Scope
	// trust resolves a team vault's members; nil for the personal vault.
	trust *memberTrust
	// landed dates each base record by the signed time of the head that carried it: the clock a
	// tombstone's trash window counts from.
	landed map[vaultsync.ID]uint64
	// preferRemote applies the account's versions over local ones without merging: the user chose
	// to replace this device's data with the account's.
	preferRemote bool
}

// Pull brings the local vault up to the server's head of one vault. Commits are verified and
// applied one at a time, each in its own vault transaction followed by the state advance, so an
// interrupted pull resumes from the last commit it finished. A commit that fails verification
// stops the pull with ErrIntegrity before any of its records reaches the vault.
func (e *SyncEngine) Pull(ctx context.Context, vault vaultsync.ID) (PullReport, error) {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return PullReport{}, err
	}
	err = p.pullAll(ctx)
	return p.report, err
}

func (p *pullRun) pullAll(ctx context.Context) error {
	if p.state.Cursor().Seq == 0 {
		if err := p.bootstrap(ctx); err != nil {
			return err
		}
	}
	resynced := false
	for {
		commits, more, err := p.e.d.Server.Changes(ctx, p.vault, p.state.Cursor().Seq, pullPageSize)
		// Resynced at most once per pull: a server that keeps claiming pruned history after handing
		// over a snapshot is stalling, and the next round asks again.
		if errors.Is(err, synctrust.ErrHistoryPruned) {
			if resynced {
				return nil
			}
			resynced = true
			if err := p.resync(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		for _, c := range commits {
			if err := p.applyCommit(ctx, c); err != nil {
				return err
			}
		}
		// An empty page that claims more would loop forever; the next pull asks again.
		if !more || len(commits) == 0 {
			return nil
		}
	}
}

func (e *SyncEngine) startPull(ctx context.Context, vault vaultsync.ID) (*pullRun, error) {
	if err := e.requireJoined(ctx, vault); err != nil {
		return nil, err
	}
	dl, fresh, err := e.verifiedDeviceLog(ctx)
	if err != nil {
		return nil, err
	}
	vt, err := e.vaultTrust(ctx, vault, dl)
	if err != nil {
		return nil, err
	}
	resolve, check, ops := vt.resolve, vt.check, vt.ops
	stored, err := e.d.State.LoadVault(ctx, vault)
	if err != nil {
		return nil, err
	}
	state, err := vaultsync.RestoreVaultState(vault, stored.Head, stored.Records)
	if err != nil {
		return nil, integrity("stored vault state", err)
	}
	p := &pullRun{e: e, vault: vault, ops: ops, resolve: resolve, state: state,
		members: &membershipCursor{vault: vault, ops: ops, check: check},
		base:    indexEnvelopes(stored.Records), names: map[vaultsync.ID]syncKeyName{},
		bodies: map[vaultsync.ID]synctrust.Body{}, landed: map[vaultsync.ID]uint64{}, scope: e.scopeOf(vault), trust: vt.members}
	for id, at := range stored.LandedAt {
		p.landed[id] = at
	}
	p.report.NewDevices = newDevices(fresh, e.d.Keys.Device())
	p.report.MembershipSeq = len(ops)
	if meta, ok := p.base[synctrust.VaultMetaID(vault)]; ok {
		if err := p.learnMeta(meta); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// applyCommit verifies one commit, applies what may be applied, quarantines or holds the rest, and
// advances the stored state. The vault is written before the state: after a crash in between, the
// next pull applies the same commit again, and applying a version twice changes nothing.
func (p *pullRun) applyCommit(ctx context.Context, c synctrust.RemoteCommit) error {
	m, err := p.members.at(c.Commit.Head.MembershipSeq)
	if err != nil {
		return err
	}
	next, err := p.state.ApplyCommit(c.Commit, m, p.resolve)
	if err != nil {
		return integrity("commit", err)
	}
	if err := p.land(ctx, c.Commit, next); err != nil {
		return err
	}
	p.report.Commits++
	return nil
}

// land applies a verified commit's records to the vault and then advances the stored state to it.
func (p *pullRun) land(ctx context.Context, c vaultsync.Commit, next *vaultsync.VaultState) error {
	return p.landAs(ctx, c, c, next)
}

// landAs stores c but applies meaning to the vault: they differ only for a resync, where records
// deleted and purged while this device was away reach the vault as deletions they never were on
// the wire.
func (p *pullRun) landAs(ctx context.Context, c, meaning vaultsync.Commit, next *vaultsync.VaultState) error {
	out, err := p.interpret(meaning)
	if err != nil {
		return err
	}
	if len(out.changes) > 0 {
		if err := p.e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return p.applyInterpretation(d, out) }); err != nil {
			return fmt.Errorf("apply synced changes: %w", err)
		}
	}
	if err := p.settleQuarantine(ctx, out); err != nil {
		return err
	}
	if err := p.e.d.State.AdvanceVault(ctx, c.Head, c.Records, c.Purged); err != nil {
		return err
	}
	p.advance(next, c)
	p.report.Applied += len(out.changes)
	return nil
}

func (p *pullRun) settleQuarantine(ctx context.Context, out interpretation) error {
	for _, id := range out.applied {
		if err := p.e.d.State.Release(ctx, p.vault, id); err != nil {
			return err
		}
	}
	for _, q := range out.quarantine {
		if err := p.e.d.State.Quarantine(ctx, q); err != nil {
			return err
		}
		if q.Pending {
			p.report.Held++
		} else {
			p.report.Quarantined++
		}
	}
	return nil
}

// advance moves the run onto a commit it verified and stored.
func (p *pullRun) advance(next *vaultsync.VaultState, c vaultsync.Commit) {
	p.state = next
	for _, r := range c.Records {
		p.base[r.Record] = r
		p.landed[r.Record] = c.Head.ClientTimeUnixMilli
		delete(p.bodies, r.Record)
	}
	for _, id := range c.Purged {
		delete(p.base, id)
		delete(p.landed, id)
		delete(p.bodies, id)
	}
}

// newDevices lists the devices a pull saw certified for the first time, other than this one, so the
// user is told when a device joins the account: the alert that catches a stolen sync password.
func newDevices(fresh []vaultsync.DeviceEntry, self vaultsync.ID) []vaultsync.DeviceCert {
	var out []vaultsync.DeviceCert
	for _, e := range fresh {
		if e.Op == vaultsync.OpAddDevice && e.Cert != nil && e.Cert.Device != self {
			out = append(out, *e.Cert)
		}
	}
	return out
}

// applyInterpretation applies a commit's changes to the vault and suspends the bindings its
// repointings call for, in one vault update.
func (p *pullRun) applyInterpretation(d *domain.VaultData, out interpretation) error {
	if err := synctrust.ApplyScope(d, p.scope, out.changes); err != nil {
		return err
	}
	for _, s := range out.suspend {
		synctrust.SuspendBindings(d, p.scope.Vault, s.connection, s.account)
	}
	return nil
}
