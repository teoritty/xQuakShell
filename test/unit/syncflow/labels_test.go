package syncflow

import (
	"slices"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/usecase"
)

// The trash lists what the person deleted, by the name they gave it. A conflict they resolved is
// the engine's bookkeeping: it used to appear there as a restorable item spelled in record ids.
func TestTheTrashShowsNamesAndHidesResolvedConflicts(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections,
			domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22},
			domain.Connection{ID: "c2", Name: "db", Host: "db.example", Port: 22})
	})
	a.sync()
	b := s.enrolled("phone", "correct horse")
	setHost := func(id, host string) func(*domain.VaultData) {
		return func(v *domain.VaultData) {
			for i := range v.Connections {
				if v.Connections[i].ID == id {
					v.Connections[i].Host = host
				}
			}
		}
	}
	a.edit(setHost("c1", "a.example"))
	b.edit(setHost("c1", "b.example"))
	a.sync()
	b.sync()
	a.sync()

	conflicts := usecase.NewSyncConflictService(a.vault)
	described, err := conflicts.Described()
	if err != nil || len(described) != 1 || described[0].ItemName != "web" {
		t.Fatalf("conflicts = %+v, %v; a conflict names its item as the person named it", described, err)
	}
	if err := conflicts.Resolve(ctx, described[0].ID, false); err != nil {
		t.Fatal(err)
	}
	a.edit(func(v *domain.VaultData) {
		v.Connections = slices.DeleteFunc(v.Connections, func(c domain.Connection) bool { return c.ID == "c2" })
	})
	a.sync()

	trash, err := a.engine.Trash(ctx, a.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].Type != synctrust.TypeConnection || trash[0].Name != "db" {
		t.Fatalf("trash = %+v; only the deleted connection, by name, and no resolved conflict", trash)
	}
}
