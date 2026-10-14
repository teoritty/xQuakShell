package syncflow

import (
	"crypto/rand"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// connectionsIn lists d's connections by the vault they are in.
func connectionsIn(d *domain.VaultData) map[string][]string {
	out := map[string][]string{}
	for _, c := range d.Connections {
		out[c.VaultID] = append(out[c.VaultID], c.Name)
	}
	return out
}

// Moving a connection between vaults and back reaches the account's other device whichever vault
// that device syncs first. The move is two commits in two vaults, and until both have arrived the
// other device holds one half: it must take them in either order without refusing a round, and end
// with the connection exactly once, in the vault it was moved to.
func TestAMoveReachesAnotherDeviceInEitherOrder(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")
	team, err := a.engine.CreateTeam(ctx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "db", Host: "db.example", Port: 22})
	})
	a.sync()
	b.sync()

	steps := []struct {
		name  string
		to    string
		order []vaultsync.ID
	}{
		{"into the team, team first", team.String(), []vaultsync.ID{team, b.session.Personal}},
		{"out again, personal first", "", []vaultsync.ID{b.session.Personal, team}},
		{"into the team, personal first", team.String(), []vaultsync.ID{b.session.Personal, team}},
		{"out again, team first", "", []vaultsync.ID{team, b.session.Personal}},
	}
	id := "c1"
	for _, st := range steps {
		if id, err = a.engine.MoveConnection(ctx, usecase.MoveConnectionRequest{Connection: id, To: st.to}); err != nil {
			t.Fatalf("%s: move: %v", st.name, err)
		}
		a.sync()
		if err := a.syncVault(team); err != nil {
			t.Fatalf("%s: A pushes the team: %v", st.name, err)
		}
		for _, v := range st.order {
			if err := b.syncVault(v); err != nil {
				t.Fatalf("%s: B syncs %s: %v; one half of a move must not stop the round", st.name, v, err)
			}
		}
		got := connectionsIn(b.data())
		if len(b.data().Connections) != 1 || len(got[st.to]) != 1 {
			t.Fatalf("%s: B holds %v; the connection is there once, in the vault it moved to", st.name, got)
		}
	}
}
