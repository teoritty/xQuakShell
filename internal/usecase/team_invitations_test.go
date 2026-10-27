package usecase_test

import (
	"crypto/ed25519"
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	srvusecase "xquakshell/internal/syncserver/usecase"
	"xquakshell/internal/usecase"
)

// invitedBy has bob create a team named "Ops", add the engine's account as an editor, and fill it
// with a connection - everything a stranger on the same server can do without asking anyone.
func invitedBy(w *syncWorld, bob *servertest.User) *servertest.ClientVault {
	w.t.Helper()
	cv := w.h.NewTeamVault(bob, servertest.Member{User: w.a, Role: vaultsync.RoleEditor})
	body, err := synctrust.NewTeamMetaBody(w.namingKey, "Ops")
	if err != nil {
		w.t.Fatal(err)
	}
	meta := w.sealIn(cv, synctrust.VaultMetaID(cv.ID), body)
	meta.Author = bob.Device
	msg, _ := meta.SigningBytes()
	meta.Sig = ed25519.Sign(bob.DSK, msg)
	conn := w.sealAs(cv, bob, synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "prod-db", Host: "203.0.113.9", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u1", Username: "root"}}})
	cv.Users[bob.Account] = bob
	cv.Accept(bob, cv.Commit(bob, meta, conn))
	return cv
}

func teamItems(t *testing.T, w *syncWorld, vault vaultsync.ID) int {
	t.Helper()
	data, _ := w.vault.GetData()
	n := 0
	for _, c := range data.Connections {
		if c.VaultID == vault.String() {
			n++
		}
	}
	return n
}

// Being added to a team is an invitation, not a sync: the team is not listed, nothing of it reaches
// the local vault, and every path that would read it in is refused until the person accepts.
func TestATeamIsNotSyncedBeforeItIsAccepted(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	vaults, err := w.engine.Vaults(syncCtx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(vaults, cv.ID) {
		t.Fatal("a team this account was only added to is listed for sync")
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); !errors.Is(err, usecase.ErrTeamNotJoined) {
		t.Fatalf("syncing a team not joined: %v, want ErrTeamNotJoined", err)
	}
	if _, err := w.engine.Members(syncCtx, cv.ID); !errors.Is(err, usecase.ErrTeamNotJoined) {
		t.Fatalf("listing the members of a team not joined: %v, want ErrTeamNotJoined", err)
	}
	if n := teamItems(t, w, cv.ID); n != 0 {
		t.Fatalf("%d of the team's connections reached the local vault before it was accepted", n)
	}
	if data, _ := w.vault.GetData(); data.Sync != nil && len(data.Sync.Contacts) != 0 {
		t.Fatalf("contacts = %+v; an invitation must not make its creator a contact", data.Sync.Contacts)
	}
}

// The invitation names who sent it, by the fingerprint the person can compare, and the team's name.
func TestInvitationsNameTheInviter(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	invs, err := w.engine.Invitations(syncCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(invs) != 1 {
		t.Fatalf("invitations = %+v; want the one team", invs)
	}
	inv := invs[0]
	if inv.Vault != cv.ID || inv.Inviter != bob.Account || inv.Role != vaultsync.RoleEditor || inv.Name != "Ops" {
		t.Fatalf("invitation = %+v; want bob's team Ops as editor", inv)
	}
	if want := vaultsync.AccountFingerprint(bob.AIK.Public().(ed25519.PublicKey)); inv.InviterFingerprint != want {
		t.Fatalf("inviter fingerprint %q, want %q", inv.InviterFingerprint, want)
	}
	if inv.InviterKnown || inv.InviterVerified {
		t.Fatal("a stranger's invitation reads as from a known contact")
	}
}

// Accepting records the join in the personal vault - which every other device of the account syncs -
// and from then on the team syncs in like any other.
func TestAcceptingAnInvitationSyncsTheTeam(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	if err := w.engine.AcceptTeam(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	vaults, _ := w.engine.Vaults(syncCtx)
	if !slices.Contains(vaults, cv.ID) {
		t.Fatal("an accepted team is not listed for sync")
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatalf("sync an accepted team: %v", err)
	}
	if n := teamItems(t, w, cv.ID); n != 1 {
		t.Fatalf("%d team connections after accepting, want 1", n)
	}
	snap, err := synctrust.Snapshot(w.namingKey, mustData(t, w))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(mapValues(snap), func(e synctrust.Entry) bool { return e.Type == synctrust.TypeTeam && e.Key == cv.ID.String() }) {
		t.Fatal("the join is not a personal record; the account's other devices would ask again")
	}
	if invs, _ := w.engine.Invitations(syncCtx, nil); len(invs) != 0 {
		t.Fatalf("invitations after accepting = %+v", invs)
	}
	if err := w.engine.AcceptTeam(syncCtx, cv.ID); !errors.Is(err, usecase.ErrNoInvitation) {
		t.Fatalf("accepting again: %v, want ErrNoInvitation", err)
	}
}

// Declining leaves the team: the server no longer lists it, and nothing of it was ever applied.
func TestDecliningAnInvitationLeavesTheTeam(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	if err := w.engine.DeclineTeam(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if role, _ := w.h.Svc.Vaults(syncCtx, w.h.Principal(w.a.Access)); slices.ContainsFunc(role, func(v srvusecase.Vault) bool { return v.ID == cv.ID }) {
		t.Fatal("the server still lists a declined team")
	}
	if invs, _ := w.engine.Invitations(syncCtx, nil); len(invs) != 0 {
		t.Fatalf("invitations after declining = %+v", invs)
	}
	if n := teamItems(t, w, cv.ID); n != 0 {
		t.Fatal("a declined team left items behind")
	}
}

// A team this device synced before joins were recorded is joined, not offered again, and is recorded
// so the account's other devices join it too.
func TestATeamSyncedBeforeJoinsWereRecordedStaysJoined(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	if err := w.engine.AcceptTeam(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Sync.Teams = nil; return nil })
	vaults, err := w.engine.Vaults(syncCtx)
	if err != nil || !slices.Contains(vaults, cv.ID) {
		t.Fatalf("vaults = %v, %v; a team this device already synced must stay joined", vaults, err)
	}
	if data := mustData(t, w); len(data.Sync.Teams) != 1 || data.Sync.Teams[0].Vault != cv.ID.String() {
		t.Fatalf("teams = %+v; the earlier join must be recorded", data.Sync.Teams)
	}
}

// Leaving a team removes the record of joining it, so the account's other devices drop it and a
// later invitation to it is asked about again.
func TestLeavingForgetsTheJoin(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	if err := w.engine.AcceptTeam(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.engine.LeaveTeam(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if data := mustData(t, w); len(data.Sync.Teams) != 0 {
		t.Fatalf("teams after leaving = %+v", data.Sync.Teams)
	}
}

// An invitation whose membership log does not verify is not shown, and cannot be accepted.
func TestAForgedInvitationIsNotShown(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := invitedBy(w, bob)
	w.server.tamperOps = func(ops []vaultsync.MemberOp) []vaultsync.MemberOp {
		out := slices.Clone(ops)
		if len(out) > 1 {
			out[1].Role = vaultsync.RoleAdmin
		}
		return out
	}
	if invs, err := w.engine.Invitations(syncCtx, nil); err != nil || len(invs) != 0 {
		t.Fatalf("invitations from a tampered log = %+v, %v; want none", invs, err)
	}
	if err := w.engine.AcceptTeam(syncCtx, cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("accepting a tampered invitation: %v, want ErrIntegrity", err)
	}
}

func mustData(t *testing.T, w *syncWorld) *domain.VaultData {
	t.Helper()
	d, err := w.vault.GetData()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mapValues[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
