package usecase_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/usecase"
)

func connByID(t *testing.T, d *domain.VaultData, id string) domain.Connection {
	t.Helper()
	i := slices.IndexFunc(d.Connections, func(c domain.Connection) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("no connection %q in %+v", id, d.Connections)
	}
	return d.Connections[i]
}

func TestMovingAConnectionIntoATeamSharesTheHostNotTheCredential(t *testing.T) {
	_, a := registered(t)
	team, err := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Identities["mine"] = domain.SSHIdentity{ID: "mine"}
		d.PluginSecrets = map[string][]byte{domain.PluginSecretRef("c1", "token"): []byte("sealed")}
		d.Connections = append(d.Connections, domain.Connection{ID: "c1", FolderID: "f-personal", Name: "db", Host: "db.example", Port: 22,
			DefaultUserID: "u1", PluginFields: map[string]string{"token": domain.PluginSecretRef("c1", "token")},
			Users: []domain.ConnectionUser{{ID: "u1", Username: "deploy", Auth: domain.AuthMethodKey,
				KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"mine"}}}}})
		return nil
	})
	id, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "c1", To: team.String()})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if id == "" || id == "c1" {
		t.Fatalf("moved id = %q; a move must give the connection a new id, or its two halves collide on the account's other devices", id)
	}
	data, _ := a.vault.GetData()
	if len(data.Connections) != 1 {
		t.Fatalf("connections = %+v; the personal copy is gone once the team holds it", data.Connections)
	}
	c := connByID(t, data, id)
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
	if data.Identities["mine"].VaultID != "" {
		t.Fatal("the key itself moved; it must stay in the personal vault")
	}
	if ref := c.PluginFields["token"]; ref != domain.PluginSecretRef(id, "token") || string(data.PluginSecrets[ref]) != "sealed" {
		t.Fatalf("plugin field = %q; the secret follows under the new id", ref)
	}
	if _, ok := data.PluginSecrets[domain.PluginSecretRef("c1", "token")]; ok {
		t.Fatal("the old secret reference stayed behind; it names a connection that no longer exists")
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

// Out of a team the binding becomes the credential again, so the connection keeps working as it
// did - except a binding another member's repointing suspended: copying it inline would send the
// credential to that new address without the confirmation the suspension asks for.
func TestMovingAConnectionOutOfATeamTakesTheBindingBack(t *testing.T) {
	for name, suspended := range map[string]bool{"in force": false, "suspended": true} {
		t.Run(name, func(t *testing.T) {
			w, cv, _ := boundTeamConnection(t)
			if suspended {
				_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
					d.Sync.Bindings[0].SuspendedBy = "carol"
					return nil
				})
			}
			id, err := w.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "tc"})
			if err != nil {
				t.Fatalf("move out: %v", err)
			}
			data, _ := w.vault.GetData()
			c := connByID(t, data, id)
			u := c.Users[0]
			if c.VaultID != "" || u.CredentialSource != "" {
				t.Fatalf("connection %+v user %+v; out of the team it is personal and names its credential itself", c, u)
			}
			if got := u.KeyAuth != nil && u.KeyAuth.IdentityIDs[0] == "mine"; got == suspended {
				t.Fatalf("user key = %+v with the binding %s", u.KeyAuth, name)
			}
			for _, b := range data.Sync.Bindings {
				if b.ConnectionID == "tc" || b.ConnectionID == id {
					t.Fatalf("binding %+v survived; a personal connection has none", b)
				}
			}
			if slices.ContainsFunc(data.Connections, func(x domain.Connection) bool { return x.VaultID == cv.ID.String() }) {
				t.Fatal("the team still holds the connection on this device")
			}
		})
	}
}

func TestMovingBetweenTeamsKeepsTheBindingWithTheConnection(t *testing.T) {
	w, cv, _ := boundTeamConnection(t)
	dave := w.h.Register("dave")
	ov := w.joinedTeam(dave, servertest.Member{User: w.a, Role: vaultsync.RoleEditor})
	ov.Users[w.a.Account] = w.b
	ov.Accept(w.b, ov.Commit(w.b, w.teamMeta(ov)))
	if _, err := w.engine.Sync(syncCtx, ov.ID); err != nil {
		t.Fatal(err)
	}
	other := ov.ID
	id, err := w.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "tc", To: other.String()})
	if err != nil {
		t.Fatalf("team to team: %v", err)
	}
	data, _ := w.vault.GetData()
	c := connByID(t, data, id)
	if c.VaultID != other.String() || c.Users[0].CredentialSource != domain.CredentialSourcePersonal {
		t.Fatalf("connection = %+v; it stays a shared host in the other team", c)
	}
	if _, err := domain.ResolveCredentials(data, &c, c.Users[0]); err != nil {
		t.Fatalf("resolve in the new team: %v; the binding follows the connection", err)
	}
	if slices.ContainsFunc(data.Sync.Bindings, func(b domain.CredentialBinding) bool { return b.VaultID == cv.ID.String() }) {
		t.Fatal("a binding to the old team's copy stayed behind")
	}
}

// A folder of the destination places the connection inside it; any other folder is refused before
// anything changes, and the order puts the moved connection after the ones already there.
func TestMovingIntoAFolderOfTheDestination(t *testing.T) {
	_, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "tf", Name: "web", VaultID: team.String()},
			domain.ConnectionFolder{ID: "pf", Name: "mine"})
		d.Connections = append(d.Connections, domain.Connection{ID: "c1", Name: "a", Host: "a", Port: 22},
			domain.Connection{ID: "t1", Name: "t", Host: "t", Port: 22, FolderID: "tf", VaultID: team.String(), Order: 4})
		return nil
	})
	for name, tc := range map[string]struct {
		folder string
		want   error
	}{"personal folder": {"pf", domain.ErrVaultMismatch}, "unknown folder": {"nope", domain.ErrFolderNotFound}} {
		if _, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "c1", To: team.String(), Folder: tc.folder}); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	if d, _ := a.vault.GetData(); connByID(t, d, "c1").VaultID != "" {
		t.Fatal("a refused move changed the connection")
	}
	id, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "c1", To: team.String(), Folder: "tf"})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := a.vault.GetData()
	if c := connByID(t, d, id); c.FolderID != "tf" || c.Order != 5 {
		t.Fatalf("connection = %+v; it goes into the team folder, after the one already there", c)
	}
}

func TestMovingNeedsAConnectionAndAVaultItMayWrite(t *testing.T) {
	_, a := registered(t)
	if _, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "nope"}); !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Fatalf("an unknown connection: %v, want ErrConnectionNotFound", err)
	}
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Connections = append(d.Connections, domain.Connection{ID: "c1", Name: "a", Host: "a", Port: 22})
		return nil
	})
	if id, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "c1"}); err != nil || id != "c1" {
		t.Fatalf("into the vault it is in: %q, %v; nothing to do, the id stays", id, err)
	}
	for name, to := range map[string]string{"the personal vault's id": a.session.Personal.String(), "not an id": "zz"} {
		if _, err := a.engine.MoveConnection(syncCtx, usecase.MoveConnectionRequest{Connection: "c1", To: to}); !errors.Is(err, usecase.ErrNotATeam) {
			t.Errorf("into %s: %v, want ErrNotATeam", name, err)
		}
	}
}

// A viewer may move nothing in or out: the server would refuse the team's half, and the move would
// have happened on this device alone.
func TestAViewerCannotMoveConnectionsInOrOut(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.joinedTeam(bob, servertest.Member{User: w.a, Role: vaultsync.RoleViewer})
	cv.Users[w.a.Account] = w.b
	cv.Users[bob.Account] = bob
	meta := w.teamMeta(cv)
	meta.Author = bob.Device
	msg, _ := meta.SigningBytes()
	meta.Sig = ed25519.Sign(bob.DSK, msg)
	cv.Accept(bob, cv.Commit(bob, meta, w.sealAs(cv, bob, synctrust.TypeConnection, "tc", domain.Connection{ID: "tc", Name: "db", Host: "db", Port: 22})))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Connections = append(d.Connections, domain.Connection{ID: "c1", Name: "a", Host: "a", Port: 22})
		return nil
	})
	for name, req := range map[string]usecase.MoveConnectionRequest{
		"in":  {Connection: "c1", To: cv.ID.String()},
		"out": {Connection: "tc"},
	} {
		if _, err := w.engine.MoveConnection(syncCtx, req); !errors.Is(err, vaultsync.ErrNotAuthorized) {
			t.Errorf("a viewer moving %s: %v, want ErrNotAuthorized", name, err)
		}
	}
	if d, _ := w.vault.GetData(); len(d.Connections) != 2 {
		t.Fatalf("connections = %+v; a refused move changes nothing", d.Connections)
	}
}
