package usecase_test

import (
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

func TestMovingAConnectionIntoATeamSharesTheHostNotTheCredential(t *testing.T) {
	_, a := registered(t)
	team, err := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Identities["mine"] = domain.SSHIdentity{ID: "mine"}
		d.Connections = append(d.Connections, domain.Connection{ID: "c1", FolderID: "f-personal", Name: "db", Host: "db.example", Port: 22,
			DefaultUserID: "u1", Users: []domain.ConnectionUser{{ID: "u1", Username: "deploy", Auth: domain.AuthMethodKey,
				KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"mine"}}}}})
		return nil
	})
	if err := a.engine.MoveConnectionToTeam(syncCtx, "c1", team); err != nil {
		t.Fatalf("move: %v", err)
	}
	data, _ := a.vault.GetData()
	c := data.Connections[0]
	if c.VaultID != team.String() || c.FolderID != "" {
		t.Fatalf("connection = %+v; it lands at the team's top level", c)
	}
	u := c.Users[0]
	if u.CredentialSource != domain.CredentialSourcePersonal || u.KeyAuth != nil {
		t.Fatalf("user = %+v; the credential reference must not travel with the shared host", u)
	}
	resolved, err := domain.ResolveCredentials(data, &c, u)
	if err != nil || resolved.KeyAuth.IdentityIDs[0] != "mine" {
		t.Fatalf("resolve = %+v, %v; the mover keeps using their own key through a binding", resolved, err)
	}
	if _, ok := data.Identities["mine"]; !ok || data.Identities["mine"].VaultID != "" {
		t.Fatal("the key itself moved; it must stay in the personal vault")
	}
	a.sync()
	if _, err := a.engine.Sync(syncCtx, team); err != nil {
		t.Fatal(err)
	}
	sv, _ := a.state.LoadVault(syncCtx, team)
	if len(sv.Records) < 2 {
		t.Fatalf("the team holds %d records; the moved connection is pushed to it", len(sv.Records))
	}
}

func TestMovingNeedsATeamAndAPersonalConnection(t *testing.T) {
	_, a := registered(t)
	if err := a.engine.MoveConnectionToTeam(syncCtx, "nope", a.session.Personal); !errors.Is(err, usecase.ErrNotATeam) {
		t.Fatalf("into the personal vault: %v, want ErrNotATeam", err)
	}
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err := a.engine.MoveConnectionToTeam(syncCtx, "nope", team); !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Fatalf("an unknown connection: %v, want ErrConnectionNotFound", err)
	}
}
