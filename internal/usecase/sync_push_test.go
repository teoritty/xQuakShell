package usecase_test

import (
	"encoding/json"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/usecase"
)

// catchUp makes device B adopt every commit it has not seen, verifying each as a client must.
func (w *syncWorld) catchUp() []vaultsync.Commit {
	w.t.Helper()
	commits, _, err := w.h.Svc.Changes(syncCtx, w.h.Principal(w.b.Access), w.cv.ID, w.cv.State.Cursor().Seq, 100)
	if err != nil {
		w.t.Fatal(err)
	}
	// The reference client resolves authors through one user per account. A's commits are signed
	// by A's device, so A stands in for the account while they are adopted.
	w.cv.Users[w.a.Account] = w.a
	defer func() { w.cv.Users[w.a.Account] = w.b }()
	out := make([]vaultsync.Commit, 0, len(commits))
	for _, c := range commits {
		w.cv.Adopt(c.Commit)
		out = append(out, c.Commit)
	}
	return out
}

func (w *syncWorld) sync() usecase.SyncReport {
	w.t.Helper()
	r, err := w.engine.Sync(syncCtx, w.cv.ID)
	if err != nil {
		w.t.Fatalf("sync: %v", err)
	}
	return r
}

func (w *syncWorld) edit(f func(*domain.VaultData)) {
	w.t.Helper()
	if err := w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { f(d); return nil }); err != nil {
		w.t.Fatal(err)
	}
}

// openAs decrypts a record as device B would.
func (w *syncWorld) openAs(e vaultsync.Envelope) synctrust.Body {
	w.t.Helper()
	pt, err := (synccrypto.Cipher{}).Open(w.vk, &e)
	if err != nil {
		w.t.Fatal(err)
	}
	b, err := synctrust.DecodeBody(pt)
	if err != nil {
		w.t.Fatal(err)
	}
	return b
}

// readyWorld is a world where A has synced once and B has adopted what A pushed back, so both
// devices start from one verified state.
func readyWorld(t *testing.T) *syncWorld {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}),
		w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22}))
	w.sync()
	w.catchUp()
	return w
}

func TestSyncPushesLocalEdits(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f2", Name: "Dev"})
	})
	if r := w.sync(); r.Pushed != 1 {
		t.Fatalf("pushed %d records, want 1", r.Pushed)
	}
	commits := w.catchUp()
	if len(commits) != 1 || len(commits[0].Records) != 1 {
		t.Fatalf("server holds %d new commits", len(commits))
	}
	var f domain.ConnectionFolder
	if err := json.Unmarshal(w.openAs(commits[0].Records[0]).Data, &f); err != nil || f.Name != "Dev" {
		t.Fatalf("pushed folder = %+v, %v", f, err)
	}
	if out, _ := w.state.Outbox(syncCtx, w.cv.ID); out != nil {
		t.Fatal("an acknowledged commit stayed in the outbox")
	}
	if r := w.sync(); r.Pushed != 0 || r.Pull.Commits != 0 {
		t.Fatalf("a sync with nothing new did %+v; its own commit must not come back as a change", r)
	}
}

func TestSyncPushesDeletions(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Folders = nil })
	w.sync()
	commits := w.catchUp()
	if len(commits) != 1 || len(commits[0].Records) != 1 || !commits[0].Records[0].Deleted {
		t.Fatalf("commits = %+v; a local deletion must push one tombstone", commits)
	}
}

func TestConcurrentEditsMergeOnRebase(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Connections[0].Name = "web-a" })
	w.server.beforeCommit = func() {
		w.catchUp()
		w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22, Tags: []string{"b"}}))
	}
	w.sync()
	d, _ := w.vault.GetData()
	c := d.Connections[0]
	if c.Name != "web-a" || len(c.Tags) != 1 {
		t.Fatalf("connection = %+v; both devices' edits must survive the rebase", c)
	}
	if commits := w.catchUp(); len(commits) == 0 {
		t.Fatal("the merged version was never pushed")
	}
}

func TestCredentialConflictKeepsBothValues(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Connections[0].Host = "mine.example" })
	w.server.beforeCommit = func() {
		w.catchUp()
		w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "theirs.example", Port: 22}))
	}
	w.sync()
	d, _ := w.vault.GetData()
	if d.Connections[0].Host != "theirs.example" {
		t.Fatalf("host = %q; the value the server accepted first stays in force", d.Connections[0].Host)
	}
	if d.Sync == nil || len(d.Sync.Conflicts) != 1 || string(d.Sync.Conflicts[0].Local) != `"mine.example"` {
		t.Fatalf("conflicts = %+v; the losing host must be kept for a person to choose", d.Sync)
	}
	var sawConflict bool
	for _, c := range w.catchUp() {
		for _, r := range c.Records {
			if !r.Deleted && w.openAs(r).Type == synctrust.TypeConflict {
				sawConflict = true
			}
		}
	}
	if !sawConflict {
		t.Fatal("the conflict record was not pushed; other devices would never see it")
	}
}

func TestLostReplyIsResentNotDuplicated(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Folders[0].Name = "Production" })
	w.server.dropReply = true
	if _, err := w.engine.Sync(syncCtx, w.cv.ID); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	if out, _ := w.state.Outbox(syncCtx, w.cv.ID); out == nil {
		t.Fatal("the unacknowledged commit was not kept for resending")
	}
	w.sync()
	if commits := w.catchUp(); len(commits) != 1 {
		t.Fatalf("server holds %d commits for one edit; the resend must reuse the commit id", len(commits))
	}
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d after the resend", r.Pushed)
	}
}

func TestHeldChangeIsNotRevertedByPush(t *testing.T) {
	w := readyWorld(t)
	s := *domain.NewVaultData().Settings
	s.Plugins.RequireSignedPlugins = false
	sections, _ := synctrust.SplitSettings(s)
	id, _ := synctrust.RecordID(w.namingKey, synctrust.SettingsType("plugins"), "plugins")
	w.push(w.sealed(id, synctrust.Body{Type: synctrust.SettingsType("plugins"), Schema: 1, Data: sections["plugins"]}))
	if r := w.sync(); r.Pull.Held != 1 || r.Pushed != 0 {
		t.Fatalf("report = %+v; holding a change must not push the old value over it", r)
	}
}

func TestPushNeverTouchesRecordsItCannotRead(t *testing.T) {
	w := readyWorld(t)
	unknown, _ := synctrust.RecordID(w.namingKey, "gizmo", "g")
	newer, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f9")
	w.push(w.sealed(unknown, synctrust.Body{Type: "gizmo", Schema: 1, Data: json.RawMessage(`{}`)}),
		w.sealed(newer, synctrust.Body{Type: synctrust.TypeFolder, Schema: 2, Data: json.RawMessage(`{"id":"f9","name":"x","parentId":"","order":0}`)}))
	w.catchUp()
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d; records of an unknown type or a newer schema must never be deleted or rewritten", r.Pushed)
	}
}

// Releasing a quarantined record drops it from the list, not from the vault's history: a record of a
// newer schema stays one this build must neither overwrite nor delete.
func TestReleasedNewerRecordIsStillNotOverwritten(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f9", Name: "mine"})
	})
	w.sync()
	w.catchUp()
	newer, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f9")
	w.push(w.sealed(newer, synctrust.Body{Type: synctrust.TypeFolder, Schema: 2, Data: json.RawMessage(`{"id":"f9","name":"x","parentId":"","order":0}`)}))
	w.sync()
	if err := w.state.Release(syncCtx, w.cv.ID, newer); err != nil {
		t.Fatal(err)
	}
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d over a newer-schema record", r.Pushed)
	}
	w.edit(func(d *domain.VaultData) { d.Folders = d.Folders[:1] })
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d; deleting the local copy must not tombstone a newer-schema record", r.Pushed)
	}
}

func TestOutboxCommitThatLostTheRaceIsRebuilt(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Folders[0].Name = "Production" })
	w.server.beforeCommit = func() {
		w.catchUp()
		w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22, Tags: []string{"b"}}))
	}
	w.server.dropReply = true
	// The race makes the first commit a conflict, so it is never stored; dropReply then hides the
	// rebased one's reply, leaving it in the outbox.
	_, _ = w.engine.Sync(syncCtx, w.cv.ID)
	w.sync()
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d on a quiet round", r.Pushed)
	}
	d, _ := w.vault.GetData()
	if d.Folders[0].Name != "Production" || len(d.Connections[0].Tags) != 1 {
		t.Fatalf("vault = %+v / %+v", d.Folders, d.Connections)
	}
}

func TestStaleOutboxCommitIsDroppedOnConflict(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Folders[0].Name = "Production" })
	w.server.failCommit = true
	if _, err := w.engine.Sync(syncCtx, w.cv.ID); err == nil {
		t.Fatal("a commit the server never stored was reported as pushed")
	}
	w.catchUp()
	w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22, Tags: []string{"b"}}))
	if r := w.sync(); r.Pushed != 1 {
		t.Fatalf("report = %+v; a conflicting outbox commit must be dropped and the edit committed afresh", r)
	}
	if out, _ := w.state.Outbox(syncCtx, w.cv.ID); out != nil {
		t.Fatal("the stale commit is still in the outbox")
	}
}

func TestDeletingAHeldItemLocallyDoesNotPushATombstone(t *testing.T) {
	w := readyWorld(t)
	w.push(w.item(synctrust.TypeIdentity, "k1", map[string]any{"identity": domain.SSHIdentity{ID: "k1"}}))
	w.sync()
	w.catchUp()
	w.push(w.item(synctrust.TypeIdentity, "k1", map[string]any{"identity": domain.SSHIdentity{ID: "k1", AllowPlugins: true}}))
	w.sync()
	w.edit(func(d *domain.VaultData) { delete(d.Identities, "k1") })
	if r := w.sync(); r.Pushed != 0 {
		t.Fatalf("pushed %d; a held item is the user's decision, not this device's to delete everywhere", r.Pushed)
	}
}

func TestEditBeatsAConcurrentDeletion(t *testing.T) {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Folders[0].Name = "Production" })
	id, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f1")
	w.server.beforeCommit = func() {
		w.catchUp()
		w.push(w.tombstone(id))
	}
	w.sync()
	d, _ := w.vault.GetData()
	if len(d.Folders) != 1 || d.Folders[0].Name != "Production" {
		t.Fatalf("folders = %+v; an edit beats a deletion", d.Folders)
	}
	var recreated bool
	for _, c := range w.catchUp() {
		for _, r := range c.Records {
			recreated = recreated || (r.Record == id && !r.Deleted)
		}
	}
	if !recreated {
		t.Fatal("the edited item was not pushed back over the tombstone")
	}
}

func conflictWorld(t *testing.T) *syncWorld {
	w := readyWorld(t)
	w.edit(func(d *domain.VaultData) { d.Connections[0].Host = "mine.example" })
	w.server.beforeCommit = func() {
		w.catchUp()
		w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Name: "web", Host: "theirs.example", Port: 22}))
	}
	w.sync()
	return w
}

func TestResolvingAConflictWritesTheChosenValueAndSyncsIt(t *testing.T) {
	w := conflictWorld(t)
	svc := usecase.NewSyncConflictService(w.vault)
	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("conflicts = %+v, %v", list, err)
	}
	if err := svc.Resolve(syncCtx, list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	d, _ := w.vault.GetData()
	if d.Connections[0].Host != "mine.example" || len(d.Sync.Conflicts) != 0 {
		t.Fatalf("host %q, conflicts %d; the chosen value must be written and the conflict gone", d.Connections[0].Host, len(d.Sync.Conflicts))
	}
	w.sync()
	var hostPushed, conflictDeleted bool
	for _, c := range w.catchUp() {
		for _, r := range c.Records {
			if r.Deleted {
				conflictDeleted = true
				continue
			}
			var conn domain.Connection
			if b := w.openAs(r); b.Type == synctrust.TypeConnection && json.Unmarshal(b.Data, &conn) == nil && conn.Host == "mine.example" {
				hostPushed = true
			}
		}
	}
	if !hostPushed || !conflictDeleted {
		t.Fatalf("pushed host %v, deleted conflict %v; the resolution must reach every device", hostPushed, conflictDeleted)
	}
}

func TestKeepingTheCurrentValueOnlyRemovesTheConflict(t *testing.T) {
	w := conflictWorld(t)
	svc := usecase.NewSyncConflictService(w.vault)
	list, _ := svc.List()
	if err := svc.Resolve(syncCtx, list[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if d, _ := w.vault.GetData(); d.Connections[0].Host != "theirs.example" || len(d.Sync.Conflicts) != 0 {
		t.Fatal("keeping the current value changed it, or left the conflict")
	}
	if err := svc.Resolve(syncCtx, list[0].ID, false); !errors.Is(err, usecase.ErrConflictNotFound) {
		t.Fatalf("resolving twice: %v", err)
	}
}
