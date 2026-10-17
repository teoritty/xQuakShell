//go:build e2e_sync

package synce2e

import (
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

func folderName(d *domain.VaultData, id string) string {
	for _, f := range d.Folders {
		if f.ID == id {
			return f.Name
		}
	}
	return ""
}

// S1: register on A, enrol B by password, an edit on A reaches B, and B fetches only that record.
func TestS1DeltaSync(t *testing.T) {
	s := newServer(t)
	user := unique("alice")
	a := s.registered(user, "correct horse battery")
	for _, id := range []string{"f1", "f2", "f3"} {
		a.edit(func(v *domain.VaultData) { v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: id}) })
	}
	a.sync()
	b := s.enrolled("phone", user, "correct horse battery")
	if folderName(b.data(), "f3") != "f3" {
		t.Fatal("B did not receive A's data")
	}
	a.edit(func(v *domain.VaultData) {
		for i := range v.Folders {
			if v.Folders[i].ID == "f2" {
				v.Folders[i].Name = "renamed"
			}
		}
	})
	a.sync()
	before := s.servedRecords()
	b.sync()
	if got := s.servedRecords() - before; got != 1 {
		t.Fatalf("B fetched %d records for one edit; sync moves deltas, not the vault", got)
	}
	if folderName(b.data(), "f2") != "renamed" {
		t.Fatal("the edit did not reach B")
	}
}

// S4: removing B from A ends B's access: its token and its signatures are refused, the account keys
// and the vault epoch rotate.
func TestS4Revoke(t *testing.T) {
	s := newServer(t)
	user := unique("alice")
	a := s.registered(user, "correct horse battery")
	a.sync()
	b := s.enrolled("phone", user, "correct horse battery")
	var sess usecase.SyncSession
	err := retry(func() (err error) {
		sess, err = a.svc.RevokeDevices(ctx, []vaultsync.ID{b.session.Keys.Device()}, []byte("correct horse battery"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	a.sync()
	if err := b.syncVault(b.session.Personal); err == nil {
		t.Fatal("the revoked device still syncs")
	}
	if _, err := b.svc.Relogin(ctx, []byte("correct horse battery")); !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
		t.Fatalf("relogin of the revoked device: %v", err)
	}
	ops, err := a.session.Server.Membership(ctx, a.session.Personal, 0)
	if err != nil || len(ops) == 0 {
		t.Fatalf("membership: %v", err)
	}
	if last := ops[len(ops)-1]; last.Kind != vaultsync.MemberRotateEpoch || len(last.RevokedDevices) != 1 {
		t.Fatal("the vault epoch was not rotated against the revoked device")
	}
}

// S6: a team with a verified invite, a viewer that cannot write, and a removal that ends access.
func TestS6TeamLifecycle(t *testing.T) {
	s := newServer(t)
	aliceName, bobName := unique("alice"), unique("bob")
	alice := s.registered(aliceName, "alice horse battery")
	bob := s.registered(bobName, "bob horse battery")

	var team vaultsync.ID
	if err := retry(func() (err error) { team, err = alice.engine.CreateTeam(ctx, "Ops", rand.Reader); return err }); err != nil {
		t.Fatal(err)
	}
	var inv usecase.Invitee
	if err := retry(func() (err error) { inv, err = alice.engine.LookupInvitee(ctx, bobName); return err }); err != nil {
		t.Fatal(err)
	}
	_, bobIdentity, _ := bob.session.Keys.AccountKeys()
	if inv.Fingerprint != vaultsync.AccountFingerprint(bobIdentity) {
		t.Fatal("the fingerprint the owner compares is not bob's own")
	}
	if err := retry(func() error {
		return alice.engine.Invite(ctx, usecase.InviteRequest{Vault: team, Username: bobName, Fingerprint: inv.Fingerprint,
			Role: vaultsync.RoleEditor, Verified: true})
	}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	alice.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "db", Host: "db.example", Port: 22})
	})
	if err := retry(func() error {
		_, err := alice.engine.MoveConnection(ctx, usecase.MoveConnectionRequest{Connection: "c1", To: team.String()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := alice.syncVault(team); err != nil {
		t.Fatal(err)
	}
	if err := bob.syncVault(team); err != nil {
		t.Fatalf("bob syncs the team: %v", err)
	}
	if c := bob.data().Connections; len(c) != 1 || c[0].Host != "db.example" {
		t.Fatalf("bob's connections = %+v; the member reads the team's connection", c)
	}

	bobAccount := vaultsync.ID(bob.account().Account)
	if err := retry(func() error { return alice.engine.ChangeRole(ctx, team, bobAccount, vaultsync.RoleViewer) }); err != nil {
		t.Fatal(err)
	}
	bob.edit(func(v *domain.VaultData) { v.Connections[0].Name = "mine now" })
	if err := bob.syncVault(team); err != nil {
		t.Fatalf("a viewer's round: %v", err)
	}
	if err := alice.syncVault(team); err != nil {
		t.Fatal(err)
	}
	if alice.data().Connections[0].Name != "db" {
		t.Fatal("a viewer's edit reached the team")
	}

	if err := retry(func() error { return alice.engine.RemoveMember(ctx, team, bobAccount, rand.Reader) }); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := alice.syncVault(team); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.session.Server.Membership(ctx, team, 0); !errors.Is(err, synctrust.ErrRemoteNotFound) {
		t.Fatalf("a removed member fetching the team: %v, want not found", err)
	}
}

// S11: the reply to a stored commit is lost; the client resends the same commit and the server holds
// it once.
func TestS11LostReplyAfterStore(t *testing.T) {
	s := newServer(t)
	a := s.registered(unique("alice"), "correct horse battery")
	a.sync()
	before := a.head()
	a.edit(func(v *domain.VaultData) { v.Folders = append(v.Folders, domain.ConnectionFolder{ID: "f", Name: "f"}) })
	s.dropNextCommitReply.Store(true)
	if err := a.syncVault(a.session.Personal); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	a.sync()
	if got := a.head() - before; got != 1 {
		t.Fatalf("the vault advanced %d commits for one edit; the retry must be idempotent", got)
	}
}
