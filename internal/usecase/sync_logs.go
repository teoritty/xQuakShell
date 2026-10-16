package usecase

import (
	"bytes"
	"context"
	"errors"
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
	// A device the log revokes learns so first, whatever else the log says: nothing after its own
	// revocation is for it, and the person should hear "removed", not "tampering".
	if dl.IsRevoked(e.d.Keys.Device()) {
		return nil, nil, ErrSyncDeviceRevoked
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

// errLogRolledBack is another account's device log served shorter than, or different from, the
// part of it this device verified before.
var errLogRolledBack = errors.New("the account's device log does not extend the one verified before")

// verifiedAccountLog replays another account's device log - a co-member's or an invitee's - from
// the genesis keys the server returned, and holds the server to every entry this device verified
// of it before. what names the log in an integrity error.
//
// The log must have an entry. Only the first entry, signed by the identity key a fingerprint covers
// and chained from a genesis hash over both account keys, vouches for the encryption key: with no
// entry the server could name any encryption key beside a genuine identity key, and the next invite
// or rotation would wrap a vault key to it.
//
// The stored prefix stops a rollback. Served the log as it stood before a revocation and its
// succession, this device would wrap vault keys to the encryption key the revoked device still
// holds, and accept what that device signs. A device that never saw the log has no prefix to hold
// the server to; it trusts the log as first served.
func (e *SyncEngine) verifiedAccountLog(ctx context.Context, account vaultsync.ID, ml synctrust.MemberLog, what string) (*vaultsync.DeviceLogState, error) {
	if len(ml.Entries) == 0 {
		return nil, integrity(what, fmt.Errorf("%w: no entries", vaultsync.ErrDeviceLog))
	}
	stored, err := e.d.State.DeviceLog(ctx, account)
	if err != nil {
		return nil, err
	}
	if len(ml.Entries) < len(stored) {
		return nil, integrity(what, errLogRolledBack)
	}
	log, err := vaultsync.NewDeviceLogState(account, ml.Suite, ml.IdentityKey, ml.EncryptionKey)
	if err != nil {
		return nil, integrity(what, err)
	}
	for i := range ml.Entries {
		if i < len(stored) && !sameEntry(&stored[i], &ml.Entries[i]) {
			return nil, integrity(what, errLogRolledBack)
		}
		if err := log.Apply(&ml.Entries[i]); err != nil {
			return nil, integrity(what, err)
		}
	}
	if log.PendingSuccession() {
		return nil, integrity(what, fmt.Errorf("a revocation without its succession"))
	}
	// This account's own log is stored only by verifiedDeviceLog, which replays it from the keys
	// this device holds rather than from keys the server named.
	if account == e.d.Keys.Account() {
		return log, nil
	}
	if err := e.d.State.AppendDeviceLog(ctx, ml.Entries[len(stored):]); err != nil {
		return nil, err
	}
	return log, nil
}

func sameEntry(a, b *vaultsync.DeviceEntry) bool {
	am, aerr := a.SigningBytes()
	bm, berr := b.SigningBytes()
	return aerr == nil && berr == nil && bytes.Equal(am, bm) && bytes.Equal(a.Sig, b.Sig) && bytes.Equal(a.SuccessorSig, b.SuccessorSig)
}

// explainIntegrity looks again at the device log when a round failed verification. A device
// revoked, or left behind by a key rotation, while a round was under way reads a log from before the
// change and commits from after it - a vault re-encrypted to keys it never received - and failed
// verification for a routine event on another device. If the log now says so, the round's error
// carries that reason first, so the person is told what to do; the verification failure stays in
// the error, and nothing from the round was applied either way.
func (e *SyncEngine) explainIntegrity(ctx context.Context, err error) error {
	if !errors.Is(err, synctrust.ErrIntegrity) {
		return err
	}
	if _, _, lerr := e.verifiedDeviceLog(ctx); errors.Is(lerr, ErrSyncDeviceRevoked) || errors.Is(lerr, synctrust.ErrKeysRotated) {
		return errors.Join(lerr, err)
	}
	return err
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
// observe, when set, sees each op after it verified: a team vault's trust learns its anchors from it.
func (e *SyncEngine) verifiedMembership(ctx context.Context, vault vaultsync.ID, check vaultsync.SignatureCheck, observe func(*vaultsync.MemberOp)) ([]vaultsync.MemberOp, error) {
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
		if observe != nil {
			observe(&ops[i])
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
		if observe != nil {
			observe(&fresh[i])
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
