package usecase_test

import (
	"crypto/rand"
	"errors"
	"testing"
	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

func trash(t *testing.T, d *device) []usecase.TrashItem {
	t.Helper()
	items, err := d.engine.Trash(syncCtx, d.session.Personal)
	if err != nil {
		t.Fatalf("trash: %v", err)
	}
	return items
}

func TestTrashListsDeletedRecords(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1", "f2")
	a.sync()
	setClocks(1000, a)
	removeFolder(a, "f1")
	a.sync()
	items := trash(t, a)
	if len(items) != 1 {
		t.Fatalf("trash = %+v, want the one deleted folder", items)
	}
	it := items[0]
	if it.Type != synctrust.TypeFolder || it.Key != "f1" || !it.Restorable {
		t.Fatalf("item = %+v; want folder f1, restorable", it)
	}
	if it.DeletedUnixMilli != 1000 || it.PurgeUnixMilli != 1000+30*retentionDay {
		t.Fatalf("deleted %d, purged %d; want the deletion time and one window later", it.DeletedUnixMilli, it.PurgeUnixMilli)
	}
}

func TestRestoreBringsARecordBack(t *testing.T) {
	_, a, b := pair(t)
	addFolders(a, "f1")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	items := trash(t, a)
	if err := a.engine.Restore(syncCtx, a.session.Personal, items[0].Record); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !folderIDs(a)["f1"] {
		t.Fatal("the restored folder is not back in the vault")
	}
	a.sync()
	b.sync()
	if !folderIDs(b)["f1"] {
		t.Fatal("the restore did not reach the other device")
	}
	if len(trash(t, a)) != 0 {
		t.Fatal("a restored item is still in the trash")
	}
}

func TestRestoreOfSomethingNotInTheTrash(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	if err := a.engine.Restore(syncCtx, a.session.Personal, vaultsync.ID{9}); !errors.Is(err, usecase.ErrTrashNotFound) {
		t.Fatalf("err = %v, want ErrTrashNotFound", err)
	}
}

func TestShredNowEmptiesTheTrashAndRetiresOldEpochsAtOnce(t *testing.T) {
	h, a := registered(t)
	addFolders(a, "f1", "f2")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	if err := a.engine.ShredNow(syncCtx, a.session.Personal); err != nil {
		t.Fatalf("shred now: %v", err)
	}
	if _, tombs := storedRecords(t, a); tombs != 0 {
		t.Fatal("shred now left a tombstone")
	}
	if epoch := currentEpoch(t, a); epoch != 1 {
		t.Fatalf("epoch %d after shred now, want 1", epoch)
	}
	ops, _ := h.Svc.Membership(syncCtx, h.Principal(a.server.svcServer.token), a.session.Personal, 0)
	if !ops[0].Wraps[0].Pruned() {
		t.Fatal("the server still holds the old epoch's key wrap after shred now")
	}
	if !folderIDs(a)["f2"] {
		t.Fatal("shred now lost a live folder")
	}
}

func TestShredNowResumesAfterACrash(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1", "f2")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	// The rotation is stored but its reply is lost, so nothing was re-encrypted or recorded in the
	// outbox: only the state shows the shred is unfinished.
	a.server.svcServer.dropAppendReply = true
	if err := a.engine.ShredNow(syncCtx, a.session.Personal); err == nil {
		t.Fatal("shred now reported success with its rotation reply lost")
	}
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 0 {
		t.Fatal("the round after a crashed shred did not purge the trash")
	}
	sv, _ := a.state.LoadVault(syncCtx, a.session.Personal)
	for _, r := range sv.Records {
		if r.Epoch == 0 && !r.Deleted {
			t.Fatal("the round after a crashed shred left a record under the shredded epoch")
		}
	}
}

func TestTrashDoesNotOpenARetiredEpoch(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1", "f2")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	items := trash(t, a)
	kept, err := a.server.svcServer.Versions(syncCtx, a.session.Personal, items[0].Record)
	if err != nil {
		t.Fatal(err)
	}
	// Rotate with shred but do not run the round that would purge: the tombstone is still in the
	// trash, its last live version sealed under the retired epoch. A hostile server serves it anyway.
	if err := a.engine.RotateEpoch(syncCtx, a.session.Personal, nil, true, rand.Reader); err != nil {
		t.Fatal(err)
	}
	a.server.svcServer.tamperVersions = func([]vaultsync.Envelope) []vaultsync.Envelope { return kept }
	items = trash(t, a)
	if len(items) != 1 || items[0].Restorable || items[0].Key != "" {
		t.Fatalf("item = %+v; a version under a retired epoch must not be opened, whatever the server serves", items)
	}
}

func TestRestoreThatWouldWeakenThisDeviceIsRefused(t *testing.T) {
	_, a := registered(t)
	_ = a.vault.UpdateData(syncCtx, func(v *domain.VaultData) error {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "web", Host: "web.example", Port: 22,
			AgentForwarding: &agentforward.Config{Enabled: true}})
		return nil
	})
	a.sync()
	_ = a.vault.UpdateData(syncCtx, func(v *domain.VaultData) error { v.Connections = nil; return nil })
	a.sync()
	items := trash(t, a)
	if len(items) != 1 {
		t.Fatalf("trash = %+v", items)
	}
	if err := a.engine.Restore(syncCtx, a.session.Personal, items[0].Record); !errors.Is(err, usecase.ErrRestoreWeakens) {
		t.Fatalf("err = %v, want ErrRestoreWeakens: bringing back agent forwarding needs the review a held change gets", err)
	}
	if data, _ := a.vault.GetData(); len(data.Connections) != 0 {
		t.Fatal("a refused restore still wrote to the vault")
	}
}

func TestRestoreOfALiveRecordIsRefused(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	_ = a.vault.UpdateData(syncCtx, func(v *domain.VaultData) error { v.Folders[0].Name = "renamed"; return nil })
	a.sync()
	var id vaultsync.ID
	sv, _ := a.state.LoadVault(syncCtx, a.session.Personal)
	for _, r := range sv.Records {
		// The renamed folder is the only record at version 2; settings sections and vault-meta
		// were written once.
		if r.Version == 2 {
			id = r.Record
		}
	}
	if err := a.engine.Restore(syncCtx, a.session.Personal, id); !errors.Is(err, usecase.ErrTrashNotFound) {
		t.Fatalf("err = %v, want ErrTrashNotFound: restoring a live record would roll it back", err)
	}
	if data, _ := a.vault.GetData(); data.Folders[0].Name != "renamed" {
		t.Fatal("a refused restore rolled the folder back")
	}
}
