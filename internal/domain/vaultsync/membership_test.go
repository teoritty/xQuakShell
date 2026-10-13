package vaultsync_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

var (
	vaultID  = vaultsync.ID{0x7a}
	owner    = vaultsync.ID{0x01}
	admin    = vaultsync.ID{0x02}
	editor   = vaultsync.ID{0x03}
	viewer   = vaultsync.ID{0x04}
	outsider = vaultsync.ID{0x05}
)

// party is an account with one device in these tests; the device id is derived from the account.
type party struct {
	account vaultsync.ID
	device  vaultsync.ID
	key     ed25519.PrivateKey
}

func parties() map[vaultsync.ID]party {
	out := map[vaultsync.ID]party{}
	for i, acc := range []vaultsync.ID{owner, admin, editor, viewer, outsider} {
		out[acc] = party{account: acc, device: vaultsync.ID{0xd0, acc[0]}, key: testKey(byte(40 + i))}
	}
	return out
}

// checker verifies a signature against the party's device key, as a client would after resolving
// the device through the author's device log.
func checker(ps map[vaultsync.ID]party) vaultsync.SignatureCheck {
	return func(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
		p, ok := ps[account]
		if !ok || p.device != device {
			return vaultsync.ErrBadSignature
		}
		return vaultsync.VerifySignature(alg, p.key.Public().(ed25519.PublicKey), msg, sig)
	}
}

func wrap(member vaultsync.ID) vaultsync.KeyWrap {
	w, err := vaultsync.NewKeyWrap(member, bytes.Repeat([]byte{member[0]}, 1168))
	if err != nil {
		panic(err)
	}
	return w
}

type fixture struct {
	t     *testing.T
	ps    map[vaultsync.ID]party
	state *vaultsync.MembershipState
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, err := vaultsync.NewMembershipState(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, ps: parties(), state: s}
}

func (f *fixture) op(author vaultsync.ID, op *vaultsync.MemberOp) *vaultsync.MemberOp {
	f.t.Helper()
	p := f.ps[author]
	op.Vault, op.Seq, op.Prev = vaultID, f.state.Seq()+1, f.state.Hash()
	op.Author, op.AuthorDevice, op.Suite, op.SigAlg = p.account, p.device, vaultsync.Suite1, vaultsync.SigEd25519
	msg, err := op.SigningBytes()
	if err != nil {
		f.t.Fatal(err)
	}
	op.Sig = ed25519.Sign(p.key, msg)
	return op
}

func (f *fixture) apply(author vaultsync.ID, op *vaultsync.MemberOp) error {
	return f.state.Apply(f.op(author, op), checker(f.ps))
}

func (f *fixture) must(author vaultsync.ID, op *vaultsync.MemberOp) {
	f.t.Helper()
	if err := f.apply(author, op); err != nil {
		f.t.Fatalf("op %d by %x: %v", op.Kind, author[0], err)
	}
}

func (f *fixture) create() {
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: []vaultsync.KeyWrap{wrap(owner)}, RetentionDays: 30})
}

func (f *fixture) add(by, who vaultsync.ID, role vaultsync.Role) error {
	return f.apply(by, &vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: who, Role: role, Verified: true, Epoch: f.state.Epoch(), Wraps: []vaultsync.KeyWrap{wrap(who)}})
}

func (f *fixture) team() {
	f.create()
	for _, m := range []struct {
		who  vaultsync.ID
		role vaultsync.Role
	}{{admin, vaultsync.RoleAdmin}, {editor, vaultsync.RoleEditor}, {viewer, vaultsync.RoleViewer}} {
		if err := f.add(owner, m.who, m.role); err != nil {
			f.t.Fatal(err)
		}
	}
}

func wrapsFor(ids ...vaultsync.ID) []vaultsync.KeyWrap {
	out := make([]vaultsync.KeyWrap, len(ids))
	for i, id := range ids {
		out[i] = wrap(id)
	}
	return out
}

func TestMembershipCreateMakesOwner(t *testing.T) {
	f := newFixture(t)
	f.create()
	if f.state.Role(owner) != vaultsync.RoleOwner || f.state.Owner() != owner || f.state.Epoch() != 0 {
		t.Fatalf("after create: role %d, owner %x, epoch %d", f.state.Role(owner), f.state.Owner(), f.state.Epoch())
	}
	if f.state.RetentionDays() != 30 {
		t.Fatalf("retention = %d, want 30", f.state.RetentionDays())
	}
}

func TestMembershipFirstOpMustBeCreate(t *testing.T) {
	f := newFixture(t)
	if err := f.add(owner, admin, vaultsync.RoleAdmin); !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership", err)
	}
}

func TestMembershipCreateOnlyOnce(t *testing.T) {
	f := newFixture(t)
	f.create()
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: wrapsFor(owner)})
	if !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership; a second create would reset ownership", err)
	}
}

func TestMembershipCreateValidates(t *testing.T) {
	cases := map[string]*vaultsync.MemberOp{
		"nonzero epoch":  {Kind: vaultsync.MemberCreate, Epoch: 1, Wraps: wrapsFor(owner)},
		"long retention": {Kind: vaultsync.MemberCreate, RetentionDays: 366, Wraps: wrapsFor(owner)},
		"wrap for other": {Kind: vaultsync.MemberCreate, Wraps: wrapsFor(admin)},
		"unknown suite":  nil,
	}
	for name, op := range cases {
		f := newFixture(t)
		if op == nil {
			signed := f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: wrapsFor(owner)})
			signed.Suite = 99
			msg, _ := signed.SigningBytes()
			signed.Sig = ed25519.Sign(f.ps[owner].key, msg)
			if err := f.state.Apply(signed, checker(f.ps)); !errors.Is(err, vaultsync.ErrMembership) {
				t.Errorf("%s: err = %v, want ErrMembership", name, err)
			}
			continue
		}
		if err := f.apply(owner, op); !errors.Is(err, vaultsync.ErrMembership) {
			t.Errorf("%s: err = %v, want ErrMembership", name, err)
		}
	}
}

func TestMembershipRolesGateManagement(t *testing.T) {
	f := newFixture(t)
	f.team()
	if err := f.add(editor, outsider, vaultsync.RoleViewer); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("editor adding: err = %v, want ErrNotAuthorized", err)
	}
	if err := f.add(viewer, outsider, vaultsync.RoleViewer); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("viewer adding: err = %v, want ErrNotAuthorized", err)
	}
	if err := f.add(admin, outsider, vaultsync.RoleEditor); err != nil {
		t.Errorf("admin adding: %v", err)
	}
	if err := f.add(admin, vaultsync.ID{0x06}, vaultsync.RoleOwner); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("adding a second owner: err = %v, want ErrMembership", err)
	}
	if vaultsync.RoleViewer.CanWrite() || !vaultsync.RoleEditor.CanWrite() || vaultsync.RoleEditor.CanManage() || !vaultsync.RoleAdmin.CanManage() {
		t.Error("role capabilities are wrong: viewers read, editors write, admins manage")
	}
}

func TestMembershipChangeRole(t *testing.T) {
	f := newFixture(t)
	f.team()
	f.must(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberChangeRole, Subject: viewer, Role: vaultsync.RoleEditor})
	if f.state.Role(viewer) != vaultsync.RoleEditor {
		t.Fatalf("role = %d, want editor", f.state.Role(viewer))
	}
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberChangeRole, Subject: owner, Role: vaultsync.RoleViewer}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("demoting the owner: err = %v, want ErrMembership", err)
	}
	if err := f.apply(editor, &vaultsync.MemberOp{Kind: vaultsync.MemberChangeRole, Subject: viewer, Role: vaultsync.RoleAdmin}); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("editor changing roles: err = %v, want ErrNotAuthorized", err)
	}
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberChangeRole, Subject: outsider, Role: vaultsync.RoleViewer}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("changing a non-member: err = %v, want ErrMembership", err)
	}
}

func TestMembershipAddNeedsExactlyOneWrapAtCurrentEpoch(t *testing.T) {
	f := newFixture(t)
	f.create()
	cases := map[string]*vaultsync.MemberOp{
		"no wrap":        {Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true},
		"wrong member":   {Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true, Wraps: wrapsFor(editor)},
		"two wraps":      {Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true, Wraps: wrapsFor(admin, admin)},
		"wrong epoch":    {Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true, Epoch: 1, Wraps: wrapsFor(admin)},
		"short wrap":     {Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true, Wraps: []vaultsync.KeyWrap{{Member: admin, Ciphertext: make([]byte, 10)}}},
		"already member": {Kind: vaultsync.MemberAdd, Subject: owner, Role: vaultsync.RoleAdmin, Verified: true, Wraps: wrapsFor(owner)},
		"zero subject":   {Kind: vaultsync.MemberAdd, Role: vaultsync.RoleAdmin, Verified: true, Wraps: wrapsFor(vaultsync.ID{})},
		"no role":        {Kind: vaultsync.MemberAdd, Subject: admin, Verified: true, Wraps: wrapsFor(admin)},
	}
	for name, op := range cases {
		if err := f.apply(owner, op); !errors.Is(err, vaultsync.ErrMembership) {
			t.Errorf("%s: err = %v, want ErrMembership", name, err)
		}
	}
}

func TestMembershipRequireVerifiedRefusesUnverified(t *testing.T) {
	f := newFixture(t)
	f.create()
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 30, RequireVerified: true})
	if !f.state.RequireVerified() {
		t.Fatal("policy did not record RequireVerified")
	}
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Wraps: wrapsFor(admin)})
	if !errors.Is(err, vaultsync.ErrUnverifiedMember) {
		t.Fatalf("err = %v, want ErrUnverifiedMember", err)
	}
}

func TestMembershipRemovalForcesRotation(t *testing.T) {
	f := newFixture(t)
	f.team()
	f.must(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: editor})
	if !f.state.PendingRotation() {
		t.Fatal("removal did not mark a rotation as pending")
	}
	if err := f.add(owner, outsider, vaultsync.RoleViewer); !errors.Is(err, vaultsync.ErrRotationRequired) {
		t.Fatalf("err = %v, want ErrRotationRequired; the removed member still holds the current key", err)
	}
	withRemoved := &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner, admin, viewer, editor)}
	if err := f.apply(owner, withRemoved); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("rotation wrapping the removed member: err = %v, want ErrMembership", err)
	}
	missing := &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner, admin)}
	if err := f.apply(owner, missing); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("rotation missing a member: err = %v, want ErrMembership", err)
	}
	skip := &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 2, Wraps: wrapsFor(owner, admin, viewer)}
	if err := f.apply(owner, skip); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("rotation skipping an epoch: err = %v, want ErrMembership", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner, admin, viewer), TimeUnixMilli: 500})
	if f.state.Epoch() != 1 || f.state.Role(editor) != vaultsync.RoleNone || f.state.PendingRotation() {
		t.Fatalf("after rotation: epoch %d, editor role %d, pending %v", f.state.Epoch(), f.state.Role(editor), f.state.PendingRotation())
	}
	rots := f.state.Rotations()
	if len(rots) != 2 || rots[1] != (vaultsync.Rotation{Epoch: 1, StartedUnixMilli: 500}) {
		t.Fatalf("rotations = %+v; epoch 0 at create and epoch 1 at 500 expected", rots)
	}
	if got := f.state.Members(); len(got) != 3 || got[0] != owner || got[1] != admin || got[2] != viewer {
		t.Fatalf("members = %v, want owner, admin, viewer in id order", got)
	}
}

func TestMembershipRotationNeedsManager(t *testing.T) {
	f := newFixture(t)
	f.team()
	err := f.apply(editor, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner, admin, editor, viewer)})
	if !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Fatalf("err = %v, want ErrNotAuthorized", err)
	}
}

func TestMembershipRemovalRights(t *testing.T) {
	f := newFixture(t)
	f.team()
	if err := f.apply(editor, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: viewer}); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("editor removing another: err = %v, want ErrNotAuthorized", err)
	}
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: owner}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("removing the owner: err = %v, want ErrMembership", err)
	}
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: outsider}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("removing a non-member: err = %v, want ErrMembership", err)
	}
	if err := f.apply(viewer, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: viewer}); err != nil {
		t.Errorf("leaving on one's own: %v", err)
	}
}

func TestMembershipRotationRevokesDevices(t *testing.T) {
	f := newFixture(t)
	f.team()
	editorDevice := f.ps[editor].device
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner, admin, editor, viewer), RevokedDevices: []vaultsync.ID{editorDevice}})
	if !f.state.IsDeviceRevoked(editorDevice) {
		t.Fatal("rotation did not record the revoked device")
	}
	if f.state.IsDeviceRevoked(f.ps[viewer].device) {
		t.Fatal("a device not listed was reported revoked")
	}
	err := f.apply(editor, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: editor})
	if !errors.Is(err, vaultsync.ErrRevokedDevice) {
		t.Fatalf("err = %v, want ErrRevokedDevice", err)
	}
}

func TestMembershipRewrap(t *testing.T) {
	f := newFixture(t)
	f.team()
	f.must(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: editor, Wraps: wrapsFor(editor)})
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: outsider, Wraps: wrapsFor(outsider)}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("rewrap for a non-member: err = %v, want ErrMembership", err)
	}
	if err := f.apply(editor, &vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: editor, Wraps: wrapsFor(editor)}); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("rewrap by an editor: err = %v, want ErrNotAuthorized", err)
	}
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberRewrap, Subject: editor, Epoch: 3, Wraps: wrapsFor(editor)}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("rewrap at another epoch: err = %v, want ErrMembership", err)
	}
}

func TestMembershipTransferOwnership(t *testing.T) {
	f := newFixture(t)
	f.team()
	if err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberTransferOwnership, Subject: outsider}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("transfer to a non-member: err = %v, want ErrMembership", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberTransferOwnership, Subject: admin})
	if f.state.Owner() != admin || f.state.Role(admin) != vaultsync.RoleOwner || f.state.Role(owner) != vaultsync.RoleAdmin {
		t.Fatalf("after transfer: owner %x, roles %d/%d", f.state.Owner(), f.state.Role(admin), f.state.Role(owner))
	}
	if err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 10}); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Fatalf("former owner setting policy: err = %v, want ErrNotAuthorized", err)
	}
}

func TestMembershipDeleteEndsTheLog(t *testing.T) {
	f := newFixture(t)
	f.team()
	if err := f.apply(admin, &vaultsync.MemberOp{Kind: vaultsync.MemberDeleteVault}); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Errorf("admin deleting: err = %v, want ErrNotAuthorized", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberDeleteVault})
	if !f.state.Deleted() {
		t.Fatal("Deleted = false after delete-vault")
	}
	if err := f.add(owner, outsider, vaultsync.RoleViewer); !errors.Is(err, vaultsync.ErrVaultDeleted) {
		t.Fatalf("err = %v, want ErrVaultDeleted", err)
	}
}

func TestMembershipPolicyBounds(t *testing.T) {
	f := newFixture(t)
	f.create()
	if err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 366}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership for retention above the maximum", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 365})
	if f.state.RetentionDays() != 365 {
		t.Fatalf("retention = %d, want 365 (the maximum itself is allowed)", f.state.RetentionDays())
	}
}

func TestMembershipUnknownKind(t *testing.T) {
	f := newFixture(t)
	f.create()
	if err := f.apply(owner, &vaultsync.MemberOp{Kind: 77}); !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership", err)
	}
}

func TestMembershipFailedApplyLeavesStateUnchanged(t *testing.T) {
	f := newFixture(t)
	f.team()
	seq, hash, epoch := f.state.Seq(), f.state.Hash(), f.state.Epoch()
	_ = f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner)})
	if f.state.Seq() != seq || f.state.Hash() != hash || f.state.Epoch() != epoch || f.state.Role(admin) != vaultsync.RoleAdmin {
		t.Fatal("a rejected op changed the state")
	}
	_ = f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: outsider})
	if f.state.PendingRotation() {
		t.Fatal("a rejected removal left a rotation pending")
	}
}

func TestMembershipRejectsBadSignatureAndPosition(t *testing.T) {
	f := newFixture(t)
	f.create()
	op := f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 5})
	op.Sig[0] ^= 1
	if err := f.state.Apply(op, checker(f.ps)); !errors.Is(err, vaultsync.ErrBadSignature) {
		t.Errorf("bad signature: err = %v", err)
	}
	op = f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 5})
	op.Prev[0] ^= 1
	if err := f.state.Apply(op, checker(f.ps)); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("wrong prev: err = %v", err)
	}
	op = f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 5})
	op.Vault[0] ^= 1
	if err := f.state.Apply(op, checker(f.ps)); !errors.Is(err, vaultsync.ErrMembership) {
		t.Errorf("wrong vault: err = %v", err)
	}
}
