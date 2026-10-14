package syncflow

import (
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// noticed lists the devices a device announces, read the way the controller reads them: from the
// stored device log and the vault's acknowledgements.
func noticed(t *testing.T, d *device) []string {
	t.Helper()
	c := usecase.NewSyncController(usecase.SyncControllerDeps{Vault: d.vault, State: d.state})
	list, err := c.NewDevices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range list {
		out = append(out, n.Name)
	}
	return out
}

// A device joining is how a stolen sync password shows, so the other devices must keep saying so
// until the person looks. The notice used to live in one round's report: whichever call fetched the
// device log first - here the trash listing the settings page runs when it opens - swallowed it,
// and the next round had nothing left to report.
func TestANewDeviceStaysAnnouncedWhoeverFetchesTheLogFirst(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")

	if _, err := a.engine.Trash(ctx, a.session.Personal); err != nil {
		t.Fatal(err)
	}
	a.sync()
	a.sync()
	if got := noticed(t, a); len(got) != 1 || got[0] != "phone" {
		t.Fatalf("A's notices = %v; the device that joined is announced and stays", got)
	}
	if got := noticed(t, b); len(got) != 0 {
		t.Fatalf("B's notices = %v; a device that joined later is told nothing about the ones before it", got)
	}

	c := usecase.NewSyncController(usecase.SyncControllerDeps{Vault: a.vault, State: a.state})
	if err := c.AcknowledgeNewDevices(ctx, []vaultsync.ID{b.session.Keys.Device()}); err != nil {
		t.Fatal(err)
	}
	if got := noticed(t, a); len(got) != 0 {
		t.Fatalf("A's notices after acknowledging = %v", got)
	}
	tablet := s.enrolled("tablet", "correct horse")
	a.sync()
	if got := noticed(t, a); len(got) != 1 || got[0] != "tablet" {
		t.Fatalf("A's notices = %v; acknowledging one device leaves the next one announced", got)
	}
	sess, err := a.svc.RevokeDevices(ctx, []vaultsync.ID{tablet.session.Keys.Device()}, []byte("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	a.sync()
	if got := noticed(t, a); len(got) != 0 {
		t.Fatalf("A's notices after removing the device = %v; a removed device needs no more attention", got)
	}
}
