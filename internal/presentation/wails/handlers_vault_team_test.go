package wails

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// teamConnRepo holds one connection; any repository method the handlers under test do not reach
// panics through the nil embedded interface, which is the point - it would be an unexpected write.
type teamConnRepo struct {
	domain.ConnectionRepository
	conn    domain.Connection
	saved   int
	deleted int
}

func (r *teamConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	if id != r.conn.ID {
		return nil, domain.ErrConnectionNotFound
	}
	c := r.conn
	return &c, nil
}

func (r *teamConnRepo) Save(_ context.Context, c *domain.Connection) error {
	r.saved++
	r.conn = *c
	return nil
}

func (r *teamConnRepo) Delete(context.Context, string) error {
	r.deleted++
	return nil
}

type noPasswords struct{ domain.PasswordRepository }
type noIdentities struct{ domain.IdentityRepository }

func viewerAPI(writable bool) (*AppAPI, *teamConnRepo) {
	a, teams, _ := teamAPI()
	teams.writable = writable
	repo := &teamConnRepo{conn: domain.Connection{ID: "tc", Name: "db", Host: "db.example", Port: 22, VaultID: hexV}}
	a.vaultSvc = usecase.NewVaultService(usecase.VaultServiceConfig{ConnRepo: repo, PasswordRepo: noPasswords{}, IdentRepo: noIdentities{}})
	return a, repo
}

func TestAViewerCannotChangeATeamConnection(t *testing.T) {
	a, repo := viewerAPI(false)
	got, err := a.SaveConnection(ConnectionDTO{ID: "tc", Name: "renamed", Host: "evil.example", Port: 22})
	if err != nil {
		t.Fatalf("save: %v; a viewer's save stores what is its own and reports the record as it stands", err)
	}
	if repo.saved != 0 || got.Name != "db" || got.Host != "db.example" {
		t.Fatalf("saved %d times, returned %+v; a viewer's edit must not reach the shared record", repo.saved, got)
	}
	if err := a.DeleteConnection("tc"); !errors.Is(err, vaultsync.ErrNotAuthorized) || repo.deleted != 0 {
		t.Fatalf("delete: %v (deleted %d); a viewer's delete is refused, not done in its vault alone", err, repo.deleted)
	}
	if _, err := a.DuplicateConnection("tc", "copy"); !errors.Is(err, vaultsync.ErrNotAuthorized) {
		t.Fatalf("duplicate: %v; a viewer cannot add to the team", err)
	}
}

func TestAnEditorChangesATeamConnection(t *testing.T) {
	a, repo := viewerAPI(true)
	if _, err := a.SaveConnection(ConnectionDTO{ID: "tc", Name: "renamed", Host: "db.example", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if repo.saved != 1 || repo.conn.Name != "renamed" {
		t.Fatalf("saved %d times, record %+v; a role that writes changes the record", repo.saved, repo.conn)
	}
	if err := a.DeleteConnection("tc"); err != nil || repo.deleted != 1 {
		t.Fatalf("delete: %v (deleted %d)", err, repo.deleted)
	}
}

func TestAPersonalConnectionIsNeverViewOnly(t *testing.T) {
	a, repo := viewerAPI(false)
	repo.conn.VaultID = ""
	if _, err := a.SaveConnection(ConnectionDTO{ID: "tc", Name: "renamed", Host: "db.example", Port: 22}); err != nil || repo.saved != 1 {
		t.Fatalf("save: %v (saved %d); a viewer role in some team says nothing about the personal vault", err, repo.saved)
	}
}
