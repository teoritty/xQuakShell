package syncflow

import (
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/usecase"
)

// registeredAs is a device with a fresh account of its own name.
func (s *server) registeredAs(name, password string) *device {
	s.t.Helper()
	d := s.device(name)
	c := d.creds(password)
	c.Username, c.Invite = name, s.h.Invite()
	sess, err := d.svc.Register(ctx, c)
	if err != nil {
		s.t.Fatalf("register %s: %v", name, err)
	}
	d.use(sess)
	return d
}

func (d *device) syncVault(v vaultsync.ID) error {
	_, err := d.engine.Sync(ctx, v)
	return err
}

// S6: an owner creates a team and invites a member after comparing fingerprints; the member reads
// it; as a viewer the member cannot write; once removed, the member fetches nothing and a record
// written afterwards does not open with the key the member held.
func TestS6TeamLifecycle(t *testing.T) {
	s := newServer(t)
	alice := s.registeredAs("alice", "alice horse")
	bob := s.registeredAs("bob", "bob horse")

	team, err := alice.engine.CreateTeam(ctx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := alice.engine.LookupInvitee(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	_, bobIdentity, _ := bob.session.Keys.AccountKeys()
	if inv.Fingerprint != vaultsync.AccountFingerprint(bobIdentity) {
		t.Fatal("the fingerprint the owner compares is not bob's own")
	}
	if err := alice.engine.Invite(ctx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: inv.Fingerprint,
		Role: vaultsync.RoleEditor, Verified: true}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	alice.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "db", Host: "db.example", Port: 22})
	})
	if err := alice.engine.MoveConnectionToTeam(ctx, "c1", team); err != nil {
		t.Fatal(err)
	}
	if err := alice.syncVault(team); err != nil {
		t.Fatal(err)
	}

	vaults, err := bob.engine.Vaults(ctx)
	if err != nil || len(vaults) != 2 {
		t.Fatalf("bob's vaults = %v, %v; the team is listed", vaults, err)
	}
	if err := bob.syncVault(team); err != nil {
		t.Fatalf("bob syncs the team: %v", err)
	}
	if c := bob.data().Connections; len(c) != 1 || c[0].VaultID != team.String() || c[0].Host != "db.example" {
		t.Fatalf("bob's connections = %+v; the member reads the team's connection", c)
	}
	ops, _ := bob.session.Server.Membership(ctx, team, 0)
	oldKey, err := bob.session.Keys.VaultKey(team, 0, ops)
	if err != nil {
		t.Fatalf("bob's team key: %v", err)
	}

	if err := alice.engine.ChangeRole(ctx, team, vaultsync.ID(bob.account().Account), vaultsync.RoleViewer); err != nil {
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

	if err := alice.engine.RemoveMember(ctx, team, vaultsync.ID(bob.account().Account), rand.Reader); err != nil {
		t.Fatalf("remove: %v", err)
	}
	alice.edit(func(v *domain.VaultData) { v.Connections[0].Host = "db2.example" })
	if err := alice.syncVault(team); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.session.Server.Membership(ctx, team, 0); !errors.Is(err, synctrust.ErrRemoteNotFound) {
		t.Fatalf("a removed member fetching the team: %v, want not found", err)
	}
	if vaults, _ := bob.engine.Vaults(ctx); len(vaults) != 1 || len(bob.data().Connections) != 0 {
		t.Fatalf("bob after removal: vaults %v, connections %+v; the local copy is dropped", vaults, bob.data().Connections)
	}
	sv, _ := alice.state.LoadVault(ctx, team)
	for i := range sv.Records {
		r := sv.Records[i]
		if r.Deleted {
			continue
		}
		if _, err := (synccrypto.Cipher{}).Open(oldKey, &r); err == nil {
			t.Fatalf("record %s written after the removal opens with the removed member's key", r.Record)
		}
	}
}
