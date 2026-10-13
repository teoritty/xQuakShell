package usecase_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
)

// succeed appends a key succession to u's log, revoking device first when it is not zero, and
// returns u's new encryption key.
func succeed(t *testing.T, h *servertest.Harness, u *servertest.User, device vaultsync.ID) []byte {
	t.Helper()
	var entries []vaultsync.DeviceEntry
	if !device.IsZero() {
		entries = append(entries, u.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: device}))
	}
	_, newAIK, _ := ed25519.GenerateKey(rand.Reader)
	newAEK := bytes.Repeat([]byte{4}, 1216)
	succ := vaultsync.DeviceEntry{Account: u.Account, Seq: u.Log.Seq() + 1, Prev: u.Log.Hash(), Op: vaultsync.OpSuccession,
		NewSuite: vaultsync.Suite1, NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: newAEK, SigAlg: vaultsync.SigEd25519}
	msg, _ := succ.SigningBytes()
	succ.Sig, succ.SuccessorSig = ed25519.Sign(u.AIK, msg), ed25519.Sign(newAIK, msg)
	if err := u.Log.Apply(&succ); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, succ)
	if err := h.Svc.AppendDeviceLog(syncCtx, h.Principal(u.Access), entries); err != nil {
		t.Fatalf("succession: %v", err)
	}
	u.AIK = newAIK
	return newAEK
}

func secondDevice(t *testing.T, h *servertest.Harness, u *servertest.User) vaultsync.ID {
	t.Helper()
	id := servertest.RandomID(t)
	_, dsk, _ := ed25519.GenerateKey(rand.Reader)
	if err := h.Svc.AppendDeviceLog(syncCtx, h.Principal(u.Access), []vaultsync.DeviceEntry{u.AddDeviceEntry(t, id, dsk)}); err != nil {
		t.Fatal(err)
	}
	return id
}

func syncTeam(t *testing.T, d *device, team vaultsync.ID) {
	t.Helper()
	if _, err := d.engine.Sync(syncCtx, team); err != nil {
		t.Fatalf("sync team: %v", err)
	}
}

func TestARevokedMemberDeviceMakesTheAdminRotate(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	lost := secondDevice(t, h, bob)
	succeed(t, h, bob, lost)
	syncTeam(t, a, team)
	m := serverMembership(t, h, asServerUser(t, h, a), team)
	if m.Epoch() != 1 || !m.IsDeviceRevoked(lost) {
		t.Fatalf("epoch %d, revoked %v; the admin's next sync must rotate away from the member's lost device", m.Epoch(), m.IsDeviceRevoked(lost))
	}
}

func TestAMembersNewKeyIsRewrapped(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	newAEK := succeed(t, h, bob, vaultsync.ID{})
	syncTeam(t, a, team)
	ops, err := h.Svc.Membership(syncCtx, h.Principal(asServerUser(t, h, a).Access), team, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := ops[len(ops)-1]
	want, _ := vaultsync.EncryptionDigest(newAEK)
	if last.Kind != vaultsync.MemberRewrap || last.Subject != bob.Account || last.Wraps[0].Recipient != want {
		t.Fatalf("last op %d for %x; the member's new key must get a wrap of the current epoch", last.Kind, last.Subject)
	}
	syncTeam(t, a, team)
	again, _ := h.Svc.Membership(syncCtx, h.Principal(asServerUser(t, h, a).Access), team, 0)
	if len(again) != len(ops) {
		t.Fatal("a member whose wrap is current was rewrapped again")
	}
}

func TestALeaveIsSettledByTheAdmin(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	cv := clientVaultOf(t, h, a, team, bob)
	if err := cv.AppendOps(bob, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: bob.Account}); err != nil {
		t.Fatalf("bob leaving: %v", err)
	}
	syncTeam(t, a, team)
	m := serverMembership(t, h, asServerUser(t, h, a), team)
	if m.PendingRotation() || m.Epoch() != 1 || m.Role(bob.Account) != vaultsync.RoleNone {
		t.Fatalf("pending %v epoch %d; the admin settles the rotation a leave left owed", m.PendingRotation(), m.Epoch())
	}
}

func TestATeamRotationWrapsToEveryMember(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	if err := a.engine.RotateEpoch(syncCtx, team, nil, false, rand.Reader); err != nil {
		t.Fatal(err)
	}
	ops, _ := h.Svc.Membership(syncCtx, h.Principal(asServerUser(t, h, a).Access), team, 0)
	last := ops[len(ops)-1]
	if last.Kind != vaultsync.MemberRotateEpoch || len(last.Wraps) != 2 {
		t.Fatalf("rotation wraps %d keys; every member must keep access", len(last.Wraps))
	}
	_ = bob
}

// clientVaultOf builds a reference-client view of a team the engine created, with both parties able
// to sign ops.
func clientVaultOf(t *testing.T, h *servertest.Harness, a *device, team vaultsync.ID, bob *servertest.User) *servertest.ClientVault {
	t.Helper()
	au := asServerUser(t, h, a)
	cv := &servertest.ClientVault{T: t, H: h, ID: team, Users: map[vaultsync.ID]*servertest.User{au.Account: au, bob.Account: bob}}
	cv.State, _ = vaultsync.NewVaultState(team)
	ops, err := h.Svc.Membership(syncCtx, h.Principal(bob.Access), team, 0)
	if err != nil {
		t.Fatal(err)
	}
	cv.Learn(ops...)
	return cv
}

// An owner changing their password rewraps their team vaults before the old key is gone: no other
// admin is there to do it, and without it the owner would be locked out of their own team.
func TestAnOwnersPasswordChangeKeepsTheirTeams(t *testing.T) {
	h, a, _, team := teamWithBob(t)
	s, err := a.svc.ChangePassword(syncCtx, []byte("correct horse"), []byte("new horse"))
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	a.session, a.engine = s, a.newEngine(s)
	syncTeam(t, a, team)
	if name, err := a.engine.TeamName(syncCtx, team); err != nil || name != "Ops" {
		t.Fatalf("team after the password change: %q, %v; the owner must still open it", name, err)
	}
	_ = h
}
