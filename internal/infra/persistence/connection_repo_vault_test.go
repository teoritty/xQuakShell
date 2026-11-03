package persistence

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

func teamFixture() (*memVault, *ConnectionRepo) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{{ID: "fp", Name: "Personal"}, {ID: "ft", Name: "Team", VaultID: "team"}}
	d.Connections = []domain.Connection{
		{ID: "cp", Name: "p", Host: "h", Port: 22},
		{ID: "ct", Name: "t", Host: "h", Port: 22, VaultID: "team"},
	}
	v := &memVault{data: d}
	return v, NewConnectionRepo(v)
}

func TestANewItemIsCreatedInTheVaultItWasAskedFor(t *testing.T) {
	v, r := teamFixture()
	c := &domain.Connection{Name: "n", Host: "h", Port: 22, VaultID: "team"}
	if err := r.Save(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := v.data.Connections[2].VaultID; got != "team" {
		t.Fatalf("new connection vault = %q, want team; it is created at the team's top level", got)
	}
	f := &domain.ConnectionFolder{Name: "n", VaultID: "team"}
	if err := r.SaveFolder(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if f.VaultID != "team" {
		t.Fatalf("new folder vault = %q, want team", f.VaultID)
	}
}

// A new item inside a folder takes the folder's vault, so the folder and its contents always sync to
// the same place; a request that names another vault is refused rather than half honoured.
func TestANewItemFollowsItsFolderIntoTheVault(t *testing.T) {
	v, r := teamFixture()
	if err := r.Save(context.Background(), &domain.Connection{Name: "n", Host: "h", Port: 22, FolderID: "ft"}); err != nil {
		t.Fatal(err)
	}
	if got := v.data.Connections[2].VaultID; got != "team" {
		t.Fatalf("connection created in a team folder has vault %q, want team", got)
	}
	if err := r.SaveFolder(context.Background(), &domain.ConnectionFolder{Name: "sub", ParentID: "ft"}); err != nil {
		t.Fatal(err)
	}
	if got := v.data.Folders[2].VaultID; got != "team" {
		t.Fatalf("subfolder of a team folder has vault %q, want team", got)
	}
	err := r.Save(context.Background(), &domain.Connection{Name: "n", Host: "h", Port: 22, FolderID: "fp", VaultID: "team"})
	if !errors.Is(err, domain.ErrVaultMismatch) {
		t.Fatalf("a team connection asked for in a personal folder: %v, want ErrVaultMismatch", err)
	}
}

// The vault that decides is the stored record's, not the payload's: an edit that leaves vaultId out
// of a team connection must still keep the member's own password out of the record the team gets.
func TestATeamEditKeepsTheMembersPasswordOutOfTheSharedRecord(t *testing.T) {
	v, r := teamFixture()
	v.data.Passwords = map[string]domain.PasswordBlob{"mine": {}}
	edit := &domain.Connection{ID: "ct", Name: "t", Host: "h", Port: 22, Users: []domain.ConnectionUser{
		{ID: "u", Username: "deploy", Auth: domain.AuthMethodPassword, PassAuth: &domain.PasswordAuthConfig{VaultRef: "mine"}},
	}}
	if err := r.Save(context.Background(), edit); err != nil {
		t.Fatal(err)
	}
	stored := v.data.Connections[1].Users[0]
	if stored.PassAuth != nil || stored.CredentialSource != domain.CredentialSourcePersonal {
		t.Fatalf("shared user = %+v, want personal-source with no reference", stored)
	}
	if v.data.Sync == nil || len(v.data.Sync.Bindings) != 1 || v.data.Sync.Bindings[0].PasswordID != "mine" ||
		v.data.Sync.Bindings[0].ConnectionID != "ct" || v.data.Sync.Bindings[0].VaultID != "team" {
		t.Fatalf("bindings = %+v, want this member's password bound to ct in the team vault, written with the record", v.data.Sync)
	}
}

func TestAnItemCannotBePutIntoAFolderOfAnotherVault(t *testing.T) {
	v, r := teamFixture()
	ctx := context.Background()
	cases := map[string]error{
		"edit into a team folder": r.Save(ctx, &domain.Connection{ID: "cp", Name: "p", Host: "h", Port: 22, FolderID: "ft"}),
		"edit into a personal":    r.Save(ctx, &domain.Connection{ID: "ct", Name: "t", Host: "h", Port: 22, FolderID: "fp"}),
		"move into a team folder": r.MoveToFolder(ctx, []string{"cp"}, "ft"),
		"batch with one foreign":  r.MoveToFolder(ctx, []string{"ct", "cp"}, "ft"),
		"folder under a team one": r.MoveFolder(ctx, "fp", "ft"),
		"rename into a team one":  r.SaveFolder(ctx, &domain.ConnectionFolder{ID: "fp", Name: "P", ParentID: "ft"}),
	}
	for name, err := range cases {
		if !errors.Is(err, domain.ErrVaultMismatch) {
			t.Errorf("%s: %v, want ErrVaultMismatch", name, err)
		}
	}
	if v.data.Connections[1].FolderID != "" || v.data.Connections[0].FolderID != "" || v.data.Folders[0].ParentID != "" {
		t.Fatalf("a refused move changed something: %+v %+v", v.data.Connections, v.data.Folders)
	}
	if err := r.MoveToFolder(ctx, []string{"ct"}, "ft"); err != nil || v.data.Connections[1].FolderID != "ft" {
		t.Fatalf("a team connection into its own team's folder: %v, %+v", err, v.data.Connections[1])
	}
	if err := r.MoveToFolder(ctx, []string{"ct"}, ""); err != nil || v.data.Connections[1].VaultID != "team" {
		t.Fatalf("to the top level it stays in its vault: %v, %+v", err, v.data.Connections[1])
	}
}
