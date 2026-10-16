package usecase_test

import (
	"bytes"
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

// attackerKey is an encryption key a hostile server holds the private half of.
var attackerKey = bytes.Repeat([]byte{7}, 1216)

// An empty log signs nothing, so nothing vouches for the encryption key the server named with it.
// Accepting one would let the server swap in a key of its own under a genuine identity key - one
// whose fingerprint the admin compared and found right - and receive the vault key in the invite.
func TestAnInviteeLogWithNoEntriesIsRefused(t *testing.T) {
	h, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	bob := h.Register("bob")
	a.server.svcServer.tamperLookup = func(ml synctrust.MemberLog) synctrust.MemberLog {
		ml.EncryptionKey, ml.Entries = attackerKey, nil
		return ml
	}
	err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor, Verified: true})
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a log with no entries leaves its encryption key unsigned", err)
	}
}

// grownLog certifies another device of u on the server, growing u's log by one entry.
func grownLog(t *testing.T, h *servertest.Harness, u *servertest.User) vaultsync.DeviceEntry {
	t.Helper()
	_, dsk, _ := ed25519.GenerateKey(rand.Reader)
	e := u.AddDeviceEntry(t, servertest.RandomID(t), dsk)
	if err := h.Svc.AppendDeviceLog(syncCtx, h.Principal(u.Access), []vaultsync.DeviceEntry{e}); err != nil {
		t.Fatal(err)
	}
	return e
}

// A log served shorter than this device saw it before is the log as it stood before some entry -
// a revocation and succession, say, after which the old encryption key belongs to a stolen device.
func TestAnInviteeLogRolledBackIsRefused(t *testing.T) {
	h, a, bob, _ := teamWithBob(t)
	grownLog(t, h, bob)
	if _, err := a.engine.LookupInvitee(syncCtx, "bob"); err != nil {
		t.Fatalf("the grown log was refused: %v", err)
	}
	a.server.svcServer.tamperLookup = func(ml synctrust.MemberLog) synctrust.MemberLog {
		ml.Entries = ml.Entries[:1]
		return ml
	}
	if _, err := a.engine.LookupInvitee(syncCtx, "bob"); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a log must extend the one this device verified", err)
	}
}

// The same length with another entry in place of one already seen is a fork of the log, and every
// entry in it still verifies: only the record of what was seen before can tell.
func TestAnInviteeLogForkedIsRefused(t *testing.T) {
	h, a, bob, _ := teamWithBob(t)
	grownLog(t, h, bob)
	if _, err := a.engine.LookupInvitee(syncCtx, "bob"); err != nil {
		t.Fatal(err)
	}
	_, dsk, _ := ed25519.GenerateKey(rand.Reader)
	// Bob's own key signs the fork, as a device of his could: the same first entry, another second.
	fork := &servertest.User{Account: bob.Account, AIK: bob.AIK}
	fork.Log, _ = vaultsync.NewDeviceLogState(bob.Account, vaultsync.Suite1, bob.AIK.Public().(ed25519.PublicKey), bob.AEK)
	first := fork.AddDeviceEntry(t, bob.Device, bob.DSK)
	other := fork.AddDeviceEntry(t, servertest.RandomID(t), dsk)
	a.server.svcServer.tamperLookup = func(ml synctrust.MemberLog) synctrust.MemberLog {
		ml.Entries = []vaultsync.DeviceEntry{first, other}
		return ml
	}
	if _, err := a.engine.LookupInvitee(syncCtx, "bob"); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: an entry other than the one seen at its position is a fork", err)
	}
}

// A rotation wraps the new vault key to each member's key as their log ends. Served a member's log
// from before its latest entries, it would wrap to a key that may no longer be theirs alone.
func TestARotationRefusesAMemberLogRolledBack(t *testing.T) {
	h, a, bob, team := teamWithBob(t)
	grownLog(t, h, bob)
	if err := a.engine.RotateEpoch(syncCtx, team, nil, false, rand.Reader); err != nil {
		t.Fatalf("honest rotation: %v", err)
	}
	a.server.svcServer.tamperMemberLog = func(account vaultsync.ID, ml synctrust.MemberLog) synctrust.MemberLog {
		if account == bob.Account {
			ml.Entries = ml.Entries[:1]
		}
		return ml
	}
	if err := a.engine.RotateEpoch(syncCtx, team, nil, false, rand.Reader); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a member's log must extend the one this device verified", err)
	}
}

func TestARotationRefusesAMemberLogWithNoEntries(t *testing.T) {
	_, a, bob, team := teamWithBob(t)
	a.server.svcServer.tamperMemberLog = func(account vaultsync.ID, ml synctrust.MemberLog) synctrust.MemberLog {
		if account == bob.Account {
			ml.EncryptionKey, ml.Entries = attackerKey, nil
		}
		return ml
	}
	if err := a.engine.RotateEpoch(syncCtx, team, nil, false, rand.Reader); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: the new vault key would be wrapped to the server's key", err)
	}
}
