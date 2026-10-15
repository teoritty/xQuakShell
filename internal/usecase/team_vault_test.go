package usecase_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	srvusecase "xquakshell/internal/syncserver/usecase"
	"xquakshell/internal/usecase"
)

// serverMembership replays a vault's membership log as the server holds it.
func serverMembership(t *testing.T, h *servertest.Harness, u *servertest.User, vault vaultsync.ID) *vaultsync.MembershipState {
	t.Helper()
	ops, err := h.Svc.Membership(syncCtx, h.Principal(u.Access), vault, 0)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := vaultsync.NewMembershipState(vault)
	for i := range ops {
		if err := m.Apply(&ops[i], func(vaultsync.ID, vaultsync.ID, vaultsync.SigAlg, []byte, []byte) error { return nil }); err != nil {
			t.Fatalf("op %d does not replay: %v", i+1, err)
		}
	}
	return m
}

func fingerprintOf(u *servertest.User) string {
	return vaultsync.AccountFingerprint(u.AIK.Public().(ed25519.PublicKey))
}

// teamWithBob has device A of the registered account create a team and invite bob as an editor.
func teamWithBob(t *testing.T) (*servertest.Harness, *device, *servertest.User, vaultsync.ID) {
	t.Helper()
	h, a := registered(t)
	team, err := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	bob := h.Register("bob")
	inv, err := a.engine.LookupInvitee(syncCtx, "bob")
	if err != nil || inv.Account != bob.Account || inv.Fingerprint != fingerprintOf(bob) {
		t.Fatalf("lookup = %+v, %v; the inviter must see bob's own fingerprint", inv, err)
	}
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: inv.Fingerprint, Role: vaultsync.RoleEditor, Verified: true}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	return h, a, bob, team
}

func TestCreatingATeamAndInvitingAMember(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	m := serverMembership(t, h, bob, team)
	if m.Role(bob.Account) != vaultsync.RoleEditor || m.Role(a.session.Keys.Account()) != vaultsync.RoleOwner {
		t.Fatalf("roles: bob %d, owner %d", m.Role(bob.Account), m.Role(a.session.Keys.Account()))
	}
	members, err := a.engine.Members(syncCtx, team)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %+v, %v", members, err)
	}
	vaults, _ := a.engine.Vaults(syncCtx)
	if len(vaults) != 2 {
		t.Fatalf("vaults = %v; the new team is synced", vaults)
	}
	data, _ := a.vault.GetData()
	var contact bool
	for _, c := range data.Sync.Contacts {
		contact = contact || (c.Account == bob.Account.String() && c.Verified && c.Username == "bob")
	}
	if !contact {
		t.Fatalf("contacts = %+v; a verified invite records the contact", data.Sync.Contacts)
	}
}

func TestAnInviteWhoseKeyChangedSinceTheLookupIsRefused(t *testing.T) {
	h, a := registered(t)
	team, err := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h.Register("bob")
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: "00000 00000", Role: vaultsync.RoleEditor, Verified: true}); !errors.Is(err, usecase.ErrInviteKeyChanged) {
		t.Fatalf("err = %v, want ErrInviteKeyChanged: the key invited must be the key the admin compared", err)
	}
}

func TestRemovingAMemberRotatesTheKey(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	if err := a.engine.RemoveMember(syncCtx, team, bob.Account, rand.Reader); err != nil {
		t.Fatalf("remove: %v", err)
	}
	m := serverMembership(t, h, asServerUser(t, h, a), team)
	if m.Role(bob.Account) != vaultsync.RoleNone || m.Epoch() != 1 || m.PendingRotation() {
		t.Fatalf("after removal: bob %d, epoch %d, pending %v; a removal rotates in the same append", m.Role(bob.Account), m.Epoch(), m.PendingRotation())
	}
	if _, err := h.Svc.Membership(syncCtx, h.Principal(bob.Access), team, 0); !errors.Is(err, srvusecase.ErrNotFound) {
		t.Fatalf("bob after removal: %v, want not found", err)
	}
}

func TestChangingARole(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	if err := a.engine.ChangeRole(syncCtx, team, bob.Account, vaultsync.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if m := serverMembership(t, h, bob, team); m.Role(bob.Account) != vaultsync.RoleViewer {
		t.Fatalf("bob's role = %d, want viewer", m.Role(bob.Account))
	}
}

func TestARoundReportsWhereTheMembershipLogStands(t *testing.T) {
	_, a, bob, team := teamWithBob(t)
	before, err := a.engine.Sync(syncCtx, team)
	if err != nil || before.Pull.MembershipSeq == 0 {
		t.Fatalf("first round: seq %d, %v; a team round reports its membership position", before.Pull.MembershipSeq, err)
	}
	if err := a.engine.ChangeRole(syncCtx, team, bob.Account, vaultsync.RoleViewer); err != nil {
		t.Fatal(err)
	}
	after, err := a.engine.Sync(syncCtx, team)
	if err != nil || after.Pull.MembershipSeq != before.Pull.MembershipSeq+1 {
		t.Fatalf("after a role change: seq %d -> %d, %v; a role change carries no record, only the position shows it",
			before.Pull.MembershipSeq, after.Pull.MembershipSeq, err)
	}
}

func TestTeamNameTravelsInTheVaultMeta(t *testing.T) {
	_, a, _, team := teamWithBob(t)
	members, _ := a.engine.Members(syncCtx, team)
	name, err := a.engine.TeamName(syncCtx, team)
	if err != nil || name != "Ops" || len(members) == 0 {
		t.Fatalf("name = %q, %v", name, err)
	}
	_ = synctrust.TypeVaultMeta
}

// asServerUser signs the device in through the harness, for reading what the server holds.
func asServerUser(t *testing.T, h *servertest.Harness, d *device) *servertest.User {
	t.Helper()
	data, _ := d.vault.GetData()
	acct := data.Sync.Account
	u := &servertest.User{Account: vaultsync.ID(acct.Account), Device: vaultsync.ID(acct.Device), DSK: ed25519.NewKeyFromSeed(acct.DeviceSeed)}
	u.Access = h.DeviceLogin(u, u.Device, u.DSK)
	return u
}

func storeContactFor(t *testing.T, d *device, account vaultsync.ID, digest string, verified bool) {
	t.Helper()
	err := d.vault.UpdateData(syncCtx, func(v *domain.VaultData) error {
		if v.Sync == nil {
			v.Sync = &domain.VaultSync{}
		}
		v.Sync.Contacts = append(v.Sync.Contacts, domain.SyncContact{Account: account.String(), IdentityDigest: digest, Verified: verified})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnInviteContradictingAContactIsRefused(t *testing.T) {
	h, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	bob := h.Register("bob")
	storeContactFor(t, a, bob.Account, strings.Repeat("ee", 32), true)
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor, Verified: true}); !errors.Is(err, usecase.ErrContactMismatch) {
		t.Fatalf("err = %v, want ErrContactMismatch: a key that changed since it was verified needs verifying again", err)
	}
}

func TestAnUnverifiedInviteKeepsAnEarlierVerification(t *testing.T) {
	h, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	bob := h.Register("bob")
	d := servertest.Fingerprint(bob)
	storeContactFor(t, a, bob.Account, hex.EncodeToString(d[:]), true)
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor, Verified: false}); err != nil {
		t.Fatal(err)
	}
	data, _ := a.vault.GetData()
	for _, c := range data.Sync.Contacts {
		if c.Account == bob.Account.String() && !c.Verified {
			t.Fatal("an unverified invite erased a verification the user already made")
		}
	}
}

func TestAnInviteeLogEndingInAnUnfinishedRevocationIsRefused(t *testing.T) {
	h, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	bob := h.Register("bob")
	revoke := bob.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: bob.Device})
	a.server.svcServer.tamperLookup = func(ml synctrust.MemberLog) synctrust.MemberLog {
		ml.Entries = append(ml.Entries, revoke)
		return ml
	}
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor, Verified: true}); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestRemovalReencryptsTheTeamsRecords(t *testing.T) {
	_, a, bob, team := teamWithBob(t)
	if err := a.engine.RemoveMember(syncCtx, team, bob.Account, rand.Reader); err != nil {
		t.Fatal(err)
	}
	sv, _ := a.state.LoadVault(syncCtx, team)
	for _, r := range sv.Records {
		if !r.Deleted && r.Epoch != 1 {
			t.Fatalf("record %s is still under epoch %d; a removed member's key must open nothing written or kept after", r.Record, r.Epoch)
		}
	}
}
