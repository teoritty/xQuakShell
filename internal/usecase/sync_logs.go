package usecase

import (
	"bytes"
	"context"
	"fmt"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// verifiedDeviceLog replays the account's device log from the account keys this device holds: the
// stored entries first, then whatever the server has beyond them. Only entries that replay are
// stored, so state.db never holds a log this device could not verify.
func (e *SyncEngine) verifiedDeviceLog(ctx context.Context) (*vaultsync.DeviceLogState, []vaultsync.DeviceEntry, error) {
	account := e.d.Keys.Account()
	suite, identity, encryption := e.d.Keys.AccountKeys()
	dl, err := vaultsync.NewDeviceLogState(account, suite, identity, encryption)
	if err != nil {
		return nil, nil, err
	}
	stored, err := e.d.State.DeviceLog(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	for i := range stored {
		if err := dl.Apply(&stored[i]); err != nil {
			return nil, nil, integrity("stored device log", err)
		}
	}
	fresh, err := e.d.Server.DeviceLog(ctx, account, dl.Seq())
	if err != nil {
		return nil, nil, err
	}
	for i := range fresh {
		if err := dl.Apply(&fresh[i]); err != nil {
			return nil, nil, integrity("device log", err)
		}
	}
	if dl.PendingSuccession() {
		return nil, nil, integrity("device log", fmt.Errorf("a revocation without its succession"))
	}
	if id, enc := e.d.Keys.CurrentKeys(); !bytes.Equal(dl.IdentityKey(), id) || !bytes.Equal(dl.EncryptionKey(), enc) {
		return nil, nil, synctrust.ErrKeysRotated
	}
	if err := e.d.State.AppendDeviceLog(ctx, fresh); err != nil {
		return nil, nil, err
	}
	return dl, fresh, nil
}

// resolver maps a device to its account and signing key through the account's device log. In the
// personal vault every author is one of this account's devices; a device the log does not certify
// resolves to nothing, and a commit it signed is refused.
func (e *SyncEngine) resolver(dl *vaultsync.DeviceLogState) vaultsync.DeviceResolver {
	account := e.d.Keys.Account()
	return func(device vaultsync.ID) (vaultsync.ID, []byte, error) {
		key, err := dl.DeviceKey(device)
		if err != nil {
			return vaultsync.ID{}, nil, err
		}
		return account, key, nil
	}
}

func signatureCheck(resolve vaultsync.DeviceResolver) vaultsync.SignatureCheck {
	return func(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
		owner, key, err := resolve(device)
		if err != nil {
			return err
		}
		if owner != account {
			return vaultsync.ErrBadSignature
		}
		return vaultsync.VerifySignature(alg, key, msg, sig)
	}
}

// verifiedMembership replays a vault's membership log the same way as the device log and returns
// every op, which the commit check needs at each head's membership position.
func (e *SyncEngine) verifiedMembership(ctx context.Context, vault vaultsync.ID, check vaultsync.SignatureCheck) ([]vaultsync.MemberOp, error) {
	ms, err := vaultsync.NewMembershipState(vault)
	if err != nil {
		return nil, err
	}
	ops, err := e.d.State.Membership(ctx, vault)
	if err != nil {
		return nil, err
	}
	for i := range ops {
		if err := ms.Apply(&ops[i], check); err != nil {
			return nil, integrity("stored membership", err)
		}
	}
	fresh, err := e.d.Server.Membership(ctx, vault, ms.Seq())
	if err != nil {
		return nil, err
	}
	for i := range fresh {
		if err := ms.Apply(&fresh[i], check); err != nil {
			return nil, integrity("membership", err)
		}
	}
	if err := e.d.State.AppendMembership(ctx, fresh); err != nil {
		return nil, err
	}
	return append(ops, fresh...), nil
}

// membershipCursor yields the membership state at a given position. Heads arrive in order and
// their membership positions never go back, so it advances incrementally.
type membershipCursor struct {
	vault vaultsync.ID
	ops   []vaultsync.MemberOp
	check vaultsync.SignatureCheck
	state *vaultsync.MembershipState
}

func (c *membershipCursor) at(seq uint64) (*vaultsync.MembershipState, error) {
	if c.state == nil || seq < c.state.Seq() {
		s, err := vaultsync.NewMembershipState(c.vault)
		if err != nil {
			return nil, err
		}
		c.state = s
	}
	if seq > uint64(len(c.ops)) {
		return nil, integrity("commit", fmt.Errorf("membership position %d beyond the log's %d", seq, len(c.ops)))
	}
	for c.state.Seq() < seq {
		if err := c.state.Apply(&c.ops[c.state.Seq()], c.check); err != nil {
			return nil, integrity("membership", err)
		}
	}
	return c.state, nil
}
