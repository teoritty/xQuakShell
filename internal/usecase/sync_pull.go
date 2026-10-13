package usecase

import (
	"context"
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
	namingKey []byte
	report    PullReport
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
	for {
		commits, more, err := e.d.Server.Changes(ctx, vault, p.state.Cursor().Seq, pullPageSize)
		if err != nil {
			return p.report, err
		}
		for _, c := range commits {
			if err := p.applyCommit(ctx, c); err != nil {
				return p.report, err
			}
		}
		// An empty page that claims more would loop forever; the next pull asks again.
		if !more || len(commits) == 0 {
			return p.report, nil
		}
	}
}

func (e *SyncEngine) startPull(ctx context.Context, vault vaultsync.ID) (*pullRun, error) {
	dl, err := e.verifiedDeviceLog(ctx)
	if err != nil {
		return nil, err
	}
	resolve := e.resolver(dl)
	check := signatureCheck(resolve)
	ops, err := e.verifiedMembership(ctx, vault, check)
	if err != nil {
		return nil, err
	}
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
		base:    indexEnvelopes(stored.Records), names: map[vaultsync.ID]syncKeyName{}}
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
	out, err := p.interpret(c.Commit)
	if err != nil {
		return err
	}
	if len(out.changes) > 0 {
		if err := p.e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return synctrust.Apply(d, out.changes) }); err != nil {
			return fmt.Errorf("apply synced changes: %w", err)
		}
	}
	if err := p.settleQuarantine(ctx, out); err != nil {
		return err
	}
	if err := p.e.d.State.AdvanceVault(ctx, c.Commit.Head, c.Commit.Records); err != nil {
		return err
	}
	p.state = next
	for _, r := range c.Commit.Records {
		p.base[r.Record] = r
	}
	p.report.Commits++
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
