package usecase_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

// clientVault is a vault as its members' devices track it: the membership and record states they
// verified, used here to build ops and commits the server must accept or refuse.
type clientVault struct {
	t     *testing.T
	h     *harness
	id    vaultsync.ID
	m     *vaultsync.MembershipState
	state *vaultsync.VaultState
	users map[vaultsync.ID]*user
	// opsSoFar are the membership ops the server accepted, in order.
	opsSoFar []vaultsync.MemberOp
}

func (cv *clientVault) resolve(device vaultsync.ID) (vaultsync.ID, []byte, error) {
	for _, u := range cv.users {
		if u.device == device {
			return u.account, u.dsk.Public().(ed25519.PublicKey), nil
		}
	}
	return vaultsync.ID{}, nil, vaultsync.ErrDeviceLog
}

func (cv *clientVault) check(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
	acct, key, err := cv.resolve(device)
	if err != nil || acct != account {
		return vaultsync.ErrBadSignature
	}
	return vaultsync.VerifySignature(alg, key, msg, sig)
}

// signOp fills and signs an op at the next position of m without applying it.
func (cv *clientVault) signOp(u *user, m *vaultsync.MembershipState, op vaultsync.MemberOp) vaultsync.MemberOp {
	cv.t.Helper()
	op.Vault, op.Seq, op.Prev = cv.id, m.Seq()+1, m.Hash()
	op.Author, op.AuthorDevice, op.Suite, op.SigAlg = u.account, u.device, vaultsync.Suite1, vaultsync.SigEd25519
	msg, err := op.SigningBytes()
	if err != nil {
		cv.t.Fatal(err)
	}
	op.Sig = ed25519.Sign(u.dsk, msg)
	return op
}

// replay rebuilds the membership state from the ops the server has accepted.
func (cv *clientVault) replay() *vaultsync.MembershipState {
	m, _ := vaultsync.NewMembershipState(cv.id)
	for i := range cv.opsSoFar {
		if err := m.Apply(&cv.opsSoFar[i], cv.check); err != nil {
			cv.t.Fatalf("an accepted op does not replay: %v", err)
		}
	}
	return m
}

// appendOps signs ops in sequence and sends them as one batch. The local state advances only when
// the server accepts, so refused batches leave the client where it was.
func (cv *clientVault) appendOps(u *user, ops ...vaultsync.MemberOp) error {
	cv.t.Helper()
	scratch := cv.replay()
	var signed []vaultsync.MemberOp
	for _, op := range ops {
		s := cv.signOp(u, scratch, op)
		_ = scratch.Apply(&s, cv.check)
		signed = append(signed, s)
	}
	if err := cv.h.svc.AppendMembership(ctx, cv.h.principal(u.access), cv.id, signed); err != nil {
		return err
	}
	cv.opsSoFar = append(cv.opsSoFar, signed...)
	cv.m = cv.replay()
	return nil
}

func wrapFor(ids ...vaultsync.ID) []vaultsync.KeyWrap {
	out := make([]vaultsync.KeyWrap, len(ids))
	for i, id := range ids {
		out[i] = vaultsync.KeyWrap{Member: id, Ciphertext: bytes.Repeat([]byte{id[0]}, 1168)}
	}
	return out
}

// record builds a signed envelope at the next version the local state expects.
func (cv *clientVault) record(u *user, record byte) vaultsync.Envelope {
	v, _ := cv.state.Version(vaultsync.ID{record})
	e := vaultsync.Envelope{Vault: cv.id, Record: vaultsync.ID{record}, Version: v + 1, Epoch: cv.m.Epoch(), Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{record}, 72), Body: bytes.Repeat([]byte{record}, 296), Author: u.device, SigAlg: vaultsync.SigEd25519}
	msg, _ := e.SigningBytes()
	e.Sig = ed25519.Sign(u.dsk, msg)
	return e
}

// commit builds a commit on the local state; it is applied locally only by accept.
func (cv *clientVault) commit(u *user, records ...vaultsync.Envelope) vaultsync.Commit {
	cv.t.Helper()
	leaves := map[vaultsync.ID]vaultsync.Leaf{}
	for _, l := range cv.state.Leaves() {
		leaves[l.Record] = l
	}
	for i := range records {
		l, _ := vaultsync.LeafOf(&records[i])
		leaves[l.Record] = l
	}
	var list []vaultsync.Leaf
	for _, l := range leaves {
		list = append(list, l)
	}
	root, _ := vaultsync.MerkleRoot(list)
	cur := cv.state.Cursor()
	h := vaultsync.Head{Vault: cv.id, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root, CurrentEpoch: cv.m.Epoch(),
		MembershipSeq: cv.m.Seq(), MembershipHead: cv.m.Hash(), Commit: randomID(cv.t), Author: u.device,
		ClientTimeUnixMilli: 1, SigAlg: vaultsync.SigEd25519}
	msg, _ := h.SigningBytes()
	h.Sig = ed25519.Sign(u.dsk, msg)
	return vaultsync.Commit{Head: h, Records: records}
}

func (cv *clientVault) push(u *user, c vaultsync.Commit) (uint64, error) {
	return cv.h.svc.Commit(ctx, cv.h.principal(u.access), cv.id, cv.state.Cursor().Seq, c)
}

func (cv *clientVault) accept(u *user, c vaultsync.Commit) uint64 {
	cv.t.Helper()
	seq, err := cv.push(u, c)
	if err != nil {
		cv.t.Fatalf("commit by %s refused: %v", u.name, err)
	}
	next, err := cv.state.ApplyCommit(c, cv.m, cv.resolve)
	if err != nil {
		cv.t.Fatalf("the server accepted a commit the client cannot verify: %v", err)
	}
	cv.state = next
	return seq
}

// newTeamVault creates a vault owned by owner and adds the others with the given roles.
func (h *harness) newTeamVault(owner *user, others map[*user]vaultsync.Role) *clientVault {
	h.t.Helper()
	cv := &clientVault{t: h.t, h: h, id: randomID(h.t), users: map[vaultsync.ID]*user{owner.account: owner}}
	cv.m, _ = vaultsync.NewMembershipState(cv.id)
	cv.state, _ = vaultsync.NewVaultState(cv.id)
	create := cv.signOp(owner, cv.m, vaultsync.MemberOp{Kind: vaultsync.MemberCreate, RetentionDays: 30, Wraps: wrapFor(owner.account)})
	if err := h.svc.CreateVault(ctx, h.principal(owner.access), create); err != nil {
		h.t.Fatalf("create vault: %v", err)
	}
	cv.opsSoFar = []vaultsync.MemberOp{create}
	cv.m = cv.replay()
	for u, role := range others {
		cv.users[u.account] = u
		add := vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: u.account, Role: role, Verified: true, Epoch: cv.m.Epoch(), Wraps: wrapFor(u.account)}
		if err := cv.appendOps(owner, add); err != nil {
			h.t.Fatalf("add %s: %v", u.name, err)
		}
	}
	return cv
}

func TestTeamVaultLifecycle(t *testing.T) {
	h := newHarness(t)
	alice, bob, carol := h.register("alice"), h.register("bob"), h.register("carol")
	cv := h.newTeamVault(alice, map[*user]vaultsync.Role{bob: vaultsync.RoleEditor})
	cv.users[carol.account] = carol
	if err := cv.appendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: carol.account, Role: vaultsync.RoleViewer,
		Verified: true, Epoch: 0, Wraps: wrapFor(carol.account)}); err != nil {
		t.Fatal(err)
	}
	if seq := cv.accept(alice, cv.commit(alice, cv.record(alice, 1), cv.record(alice, 2))); seq != 1 {
		t.Fatalf("first commit seq = %d", seq)
	}
	if seq := cv.accept(bob, cv.commit(bob, cv.record(bob, 1))); seq != 2 {
		t.Fatalf("second commit seq = %d", seq)
	}
	changes, more, err := h.svc.Changes(ctx, h.principal(carol.access), cv.id, 0, 10)
	if err != nil || more || len(changes) != 2 || len(changes[0].Commit.Records) != 2 {
		t.Fatalf("changes = %d commits, more %v, %v", len(changes), more, err)
	}
	head, records, err := h.svc.Snapshot(ctx, h.principal(carol.access), cv.id)
	if err != nil || head == nil || head.Seq != 2 || len(records) != 2 {
		t.Fatalf("snapshot: head %+v, %d records, %v", head, len(records), err)
	}
	if _, err := vaultsync.RestoreVaultState(cv.id, head, records); err != nil {
		t.Fatalf("the snapshot does not reproduce its own head: %v", err)
	}
	if _, err := cv.push(carol, cv.commit(carol, cv.record(carol, 3))); !isErr(err, usecase.ErrForbidden) {
		t.Fatalf("viewer commit: %v, want ErrForbidden", err)
	}
	vaults, _ := h.svc.Vaults(ctx, h.principal(bob.access))
	if len(vaults) != 2 {
		t.Fatalf("bob sees %d vaults, want his personal vault and the team vault", len(vaults))
	}
	if entries, err := h.svc.DeviceLog(ctx, h.principal(bob.access), alice.account, 0); err != nil || len(entries) != 1 {
		t.Fatalf("a co-member cannot read the device log it needs to verify signatures: %d, %v", len(entries), err)
	}
}

func TestCommitIsIdempotent(t *testing.T) {
	h := newHarness(t)
	alice := h.register("alice")
	cv := h.newTeamVault(alice, nil)
	c := cv.commit(alice, cv.record(alice, 1))
	seq := cv.accept(alice, c)
	again, err := h.svc.Commit(ctx, h.principal(alice.access), cv.id, 0, c)
	if err != nil || again != seq {
		t.Fatalf("a retried commit = %d, %v; want the original seq %d", again, err, seq)
	}
	changes, _, _ := h.svc.Changes(ctx, h.principal(alice.access), cv.id, 0, 10)
	if len(changes) != 1 {
		t.Fatalf("%d commits stored after a retry, want 1", len(changes))
	}
}

func TestStaleCommitConflicts(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.register("alice"), h.register("bob")
	cv := h.newTeamVault(alice, map[*user]vaultsync.Role{bob: vaultsync.RoleEditor})
	stale := cv.commit(bob, cv.record(bob, 2))
	cv.accept(alice, cv.commit(alice, cv.record(alice, 1)))
	_, err := h.svc.Commit(ctx, h.principal(bob.access), cv.id, 0, stale)
	var conflict *usecase.ConflictError
	if !isErr(err, usecase.ErrConflict) || !asConflict(err, &conflict) || conflict.CurrentSeq != 1 {
		t.Fatalf("stale commit: %v; want a conflict naming seq 1", err)
	}
}

func TestOutsidersAndRemovedMembersSeeNothing(t *testing.T) {
	h := newHarness(t)
	alice, bob, dave := h.register("alice"), h.register("bob"), h.register("dave")
	cv := h.newTeamVault(alice, map[*user]vaultsync.Role{bob: vaultsync.RoleEditor})
	cv.accept(alice, cv.commit(alice, cv.record(alice, 1)))
	pd := h.principal(dave.access)
	if _, _, err := h.svc.Changes(ctx, pd, cv.id, 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider changes: %v", err)
	}
	if _, _, err := h.svc.Snapshot(ctx, pd, cv.id); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider snapshot: %v", err)
	}
	if _, err := h.svc.Membership(ctx, pd, cv.id, 0); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider membership: %v", err)
	}
	if _, _, err := h.svc.Changes(ctx, pd, randomID(t), 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("a vault that does not exist must look the same as one the caller is not in: %v", err)
	}
	if err := cv.appendOps(alice,
		vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: bob.account},
		vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapFor(alice.account)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.Changes(ctx, h.principal(bob.access), cv.id, 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("removed member still reads the vault: %v", err)
	}
}

func TestMembershipRules(t *testing.T) {
	h := newHarness(t)
	alice, bob, carol := h.register("alice"), h.register("bob"), h.register("carol")
	cv := h.newTeamVault(alice, map[*user]vaultsync.Role{bob: vaultsync.RoleEditor})
	cv.users[carol.account] = carol
	err := cv.appendOps(bob, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: carol.account, Role: vaultsync.RoleViewer,
		Verified: true, Wraps: wrapFor(carol.account)})
	if !isErr(err, usecase.ErrForbidden) {
		t.Errorf("an editor added a member: %v, want ErrForbidden", err)
	}
	err = cv.appendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: bob.account})
	if !isErr(err, usecase.ErrMalformed) {
		t.Errorf("a removal without rotation: %v, want ErrMalformed", err)
	}
	ops, err := h.svc.Membership(ctx, h.principal(bob.access), cv.id, 1)
	if err != nil || len(ops) != 1 {
		t.Fatalf("membership after seq 1 = %d ops, %v; want only the add", len(ops), err)
	}
}

func TestCommitLimits(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.register("alice"), h.register("bob")
	cv := h.newTeamVault(alice, map[*user]vaultsync.Role{bob: vaultsync.RoleEditor})
	c := cv.commit(alice, cv.record(alice, 1))
	if _, err := h.svc.Commit(ctx, h.principal(bob.access), cv.id, 0, c); !isErr(err, usecase.ErrForbidden) {
		t.Errorf("a commit pushed by a device other than its author: %v, want ErrForbidden", err)
	}
	many := make([]vaultsync.Envelope, 1001)
	if _, err := h.svc.Commit(ctx, h.principal(alice.access), cv.id, 0, vaultsync.Commit{Head: c.Head, Records: many}); !isErr(err, usecase.ErrTooLarge) {
		t.Errorf("1001 records: %v, want ErrTooLarge", err)
	}
}

func TestQuota(t *testing.T) {
	h := newHarnessWithQuota(t, 1000)
	alice := h.register("alice")
	cv := h.newTeamVault(alice, nil)
	cv.accept(alice, cv.commit(alice, cv.record(alice, 1)))
	if _, err := cv.push(alice, cv.commit(alice, cv.record(alice, 2), cv.record(alice, 3))); !isErr(err, usecase.ErrQuotaExceeded) {
		t.Fatalf("over quota: %v, want ErrQuotaExceeded", err)
	}
}

func TestPersonalVaultCommit(t *testing.T) {
	h := newHarness(t)
	alice := h.register("alice")
	cv := &clientVault{t: t, h: h, id: alice.vault, users: map[vaultsync.ID]*user{alice.account: alice}}
	cv.m, _ = vaultsync.NewMembershipState(alice.vault)
	cv.state, _ = vaultsync.NewVaultState(alice.vault)
	ops, err := h.svc.Membership(ctx, h.principal(alice.access), alice.vault, 0)
	if err != nil || len(ops) != 1 {
		t.Fatalf("personal vault membership = %d, %v", len(ops), err)
	}
	cv.opsSoFar = ops
	cv.m = cv.replay()
	cv.accept(alice, cv.commit(alice, cv.record(alice, 1)))
}

func asConflict(err error, target **usecase.ConflictError) bool { return errors.As(err, target) }
