package usecase

import (
	"context"
	"crypto/rand"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// maintainTeam is what an admin device owes a team vault on each round (spec §5.9): a rotation when
// a member left without one or one of a member's devices was revoked on their own account, and a
// rewrap for each member whose encryption key changed since their wrap was made. It returns
// errRoundAgain when it changed the membership, so the round continues on a fresh view.
func (p *pullRun) maintainTeam(ctx context.Context, m *vaultsync.MembershipState) error {
	if p.trust == nil || !m.Role(p.e.d.Keys.Account()).CanManage() {
		return nil
	}
	revoked, err := p.unrevokedDevices(m)
	if err != nil {
		return err
	}
	if m.PendingRotation() || len(revoked) > 0 {
		op, err := p.rotationFor(m, m.Members(), rand.Reader)
		if err != nil {
			return err
		}
		op.RevokedDevices = revoked
		if err := p.appendOp(ctx, m, op); err != nil {
			return err
		}
		if err := p.e.reencrypt(ctx, p.vault); err != nil {
			return err
		}
		return errRoundAgain
	}
	rewraps, err := p.staleWraps(m)
	if err != nil || len(rewraps) == 0 {
		return err
	}
	if err := p.appendOps(ctx, rewraps...); err != nil {
		return err
	}
	return errRoundAgain
}

// unrevokedDevices lists devices members revoked on their own accounts that the vault still trusts.
// Those devices held the vault key, so the vault must rotate away from them.
func (p *pullRun) unrevokedDevices(m *vaultsync.MembershipState) ([]vaultsync.ID, error) {
	var out []vaultsync.ID
	for _, member := range m.Members() {
		log, err := p.memberLog(member)
		if err != nil {
			return nil, err
		}
		for _, d := range log.Devices() {
			if log.IsRevoked(d) && !m.IsDeviceRevoked(d) {
				out = append(out, d)
			}
		}
	}
	sortIDs(out)
	return slices.Compact(out), nil
}

func (p *pullRun) memberLog(member vaultsync.ID) (*vaultsync.DeviceLogState, error) {
	if member == p.e.d.Keys.Account() {
		return p.trust.own, nil
	}
	return p.trust.logFor(member)
}

// staleWraps builds a rewrap-member op for every member whose current encryption key is not the
// one their latest wrap of the current epoch was sealed to: after a key succession that wrap no
// longer opens for them.
func (p *pullRun) staleWraps(m *vaultsync.MembershipState) ([]vaultsync.MemberOp, error) {
	var out []vaultsync.MemberOp
	for _, member := range m.Members() {
		enc, err := p.encryptionKeyOf(member)
		if err != nil {
			return nil, err
		}
		want, err := vaultsync.EncryptionDigest(enc)
		if err != nil {
			return nil, err
		}
		if got, ok := latestWrap(p.ops, m.Epoch(), member); !ok || got.Recipient == want {
			continue
		}
		wrap, err := p.e.wrapTo(p.vault, m.Epoch(), p.ops, member, enc)
		if err != nil {
			return nil, err
		}
		out = append(out, vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: member, Epoch: m.Epoch(),
			Wraps: []vaultsync.KeyWrap{wrap}})
	}
	return out, nil
}

// latestWrap finds the last wrap of epoch made for member in the verified log.
func latestWrap(ops []vaultsync.MemberOp, epoch uint32, member vaultsync.ID) (vaultsync.KeyWrap, bool) {
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i].Epoch != epoch {
			continue
		}
		for _, w := range ops[i].Wraps {
			if w.Member == member {
				return w, true
			}
		}
	}
	return vaultsync.KeyWrap{}, false
}
