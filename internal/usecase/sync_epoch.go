package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// ErrNoCrypto is an epoch operation on an engine wired without account crypto.
var ErrNoCrypto = errors.New("sync: epoch rotation needs account crypto")

// RewrapCurrent wraps the current epoch's key to newEncryptionKey as well, before a succession
// replaces this device's encryption seed: afterwards the old seed is gone, and without this wrap
// the device could no longer open the records it must re-encrypt.
func (e *SyncEngine) RewrapCurrent(ctx context.Context, vault vaultsync.ID, newEncryptionKey []byte) error {
	if e.d.Crypto == nil {
		return ErrNoCrypto
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	vk, err := e.d.Keys.VaultKey(vault, m.Epoch(), p.ops)
	if err != nil {
		return err
	}
	defer clear(vk)
	account := e.d.Keys.Account()
	wrapped, err := e.d.Crypto.WrapVaultKey(newEncryptionKey, vault, m.Epoch(), account, vk)
	if err != nil {
		return err
	}
	wrap, err := vaultsync.NewKeyWrap(account, wrapped)
	if err != nil {
		return err
	}
	return p.appendOp(ctx, m, vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: account, Epoch: m.Epoch(),
		Wraps: []vaultsync.KeyWrap{wrap}})
}

// RotateEpoch moves the vault to a fresh key wrapped to the account's current encryption key,
// stops accepting the revoked devices from this membership position on (spec §5.4), and
// re-encrypts every live record under the new key.
func (e *SyncEngine) RotateEpoch(ctx context.Context, vault vaultsync.ID, revoked []vaultsync.ID, random io.Reader) error {
	if e.d.Crypto == nil {
		return ErrNoCrypto
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	vk := make([]byte, 32)
	if _, err := io.ReadFull(random, vk); err != nil {
		return err
	}
	defer clear(vk)
	account := e.d.Keys.Account()
	_, enc := e.d.Keys.CurrentKeys()
	wrapped, err := e.d.Crypto.WrapVaultKey(enc, vault, m.Epoch()+1, account, vk)
	if err != nil {
		return err
	}
	wrap, err := vaultsync.NewKeyWrap(account, wrapped)
	if err != nil {
		return err
	}
	// A rotation may not be dated before the previous one, so a device whose clock is behind dates
	// its rotation at the previous one instead of having it refused by every verifier.
	rotations := m.Rotations()
	op := vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: m.Epoch() + 1, Wraps: []vaultsync.KeyWrap{wrap},
		RevokedDevices: revoked, TimeUnixMilli: rotations[len(rotations)-1].StartedUnixMilli}
	if err := p.appendOp(ctx, m, op); err != nil {
		return err
	}
	return e.reencrypt(ctx, vault)
}

// caughtUp pulls a vault to the server's head and returns the run and its latest membership.
func (e *SyncEngine) caughtUp(ctx context.Context, vault vaultsync.ID) (*pullRun, *vaultsync.MembershipState, error) {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return nil, nil, err
	}
	if err := p.flushOutbox(ctx); err != nil {
		return nil, nil, err
	}
	if err := p.pullAll(ctx); err != nil {
		return nil, nil, err
	}
	m, err := p.members.at(uint64(len(p.ops)))
	return p, m, err
}

// appendOp signs one membership op at the next position after m and sends it.
func (p *pullRun) appendOp(ctx context.Context, m *vaultsync.MembershipState, op vaultsync.MemberOp) error {
	op.Vault, op.Seq, op.Prev = p.vault, m.Seq()+1, m.Hash()
	op.Author, op.AuthorDevice, op.Suite, op.SigAlg = p.e.d.Keys.Account(), p.e.d.Keys.Device(), vaultsync.Suite1, vaultsync.SigEd25519
	op.TimeUnixMilli = max(uint64(max(p.e.d.NowUnixMilli(), 0)), op.TimeUnixMilli)
	msg, err := op.SigningBytes()
	if err != nil {
		return err
	}
	if op.Sig, err = p.e.d.Keys.Sign(msg); err != nil {
		return err
	}
	if err := p.e.d.Server.AppendMembership(ctx, p.vault, []vaultsync.MemberOp{op}); err != nil {
		return fmt.Errorf("membership op %d: %w", op.Kind, err)
	}
	return nil
}

// reencrypt rewrites every live record still sealed under an older epoch, retrying when another
// device commits in between.
func (e *SyncEngine) reencrypt(ctx context.Context, vault vaultsync.ID) error {
	for attempt := 0; attempt < maxRebases; attempt++ {
		p, m, err := e.caughtUp(ctx, vault)
		if err != nil {
			return err
		}
		changes := p.staleRecords(m.Epoch())
		if len(changes) == 0 {
			return nil
		}
		err = p.commitChanges(ctx, changes)
		var conflict *synctrust.ConflictError
		if !errors.As(err, &conflict) {
			return err
		}
	}
	return ErrSyncBusy
}

// staleRecords lists live records sealed under an epoch before current, with their content and
// schema exactly as they are - including types and schemas this build cannot interpret.
func (p *pullRun) staleRecords(current uint32) []change {
	var out []change
	for id, env := range p.base {
		if env.Deleted || env.Epoch == current {
			continue
		}
		if b, ok := p.baseBody(id); ok {
			out = append(out, change{id: id, entry: synctrust.Entry{Type: b.Type, Data: b.Data}, schema: b.Schema})
		}
	}
	return out
}

// commitChanges builds, records, sends and adopts one commit of the given changes.
func (p *pullRun) commitChanges(ctx context.Context, changes []change) error {
	c, err := p.buildCommit(changes)
	if err != nil {
		return err
	}
	if err := p.e.d.State.PutOutbox(ctx, c); err != nil {
		return err
	}
	if _, err := p.e.d.Server.Commit(ctx, c); err != nil {
		var conflict *synctrust.ConflictError
		if errors.As(err, &conflict) {
			return errors.Join(err, p.e.d.State.ClearOutbox(ctx, p.vault, c.Head.Commit))
		}
		return err
	}
	return p.adopt(ctx, c)
}
