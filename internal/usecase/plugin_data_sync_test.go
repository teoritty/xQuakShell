package usecase_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/portable"
	"xquakshell/internal/usecase"
)

const testPluginID = "com.example.notes"

type fakeDataCatalog struct {
	mu        sync.Mutex
	manifests []domainplugin.Manifest
	running   map[string]bool
}

func (c *fakeDataCatalog) SyncedManifests() []domainplugin.Manifest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.manifests)
}

func (c *fakeDataCatalog) ProcessIdle(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.running[id]
}

func (c *fakeDataCatalog) setRunning(id string, running bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running[id] = running
}

type pluginDataWorld struct {
	t       *testing.T
	root    string
	vault   *syncTestVault
	catalog *fakeDataCatalog
	svc     *usecase.PluginDataSync
	changed int
}

func newPluginDataWorld(t *testing.T) *pluginDataWorld {
	t.Helper()
	w := &pluginDataWorld{t: t, root: t.TempDir()}
	d := domain.NewVaultData()
	d.Sync = &domain.VaultSync{Account: &domain.SyncAccount{Username: "u"}}
	w.vault = &syncTestVault{data: d}
	w.catalog = &fakeDataCatalog{running: map[string]bool{}, manifests: []domainplugin.Manifest{{ID: testPluginID,
		Sync: &domainplugin.SyncSpec{Paths: []string{"notes/**", "state.json"}}}}}
	w.svc = usecase.NewPluginDataSync(usecase.PluginDataSyncDeps{
		Zone:    portable.NewDataStore(w.root, "", nil),
		Vault:   w.vault,
		Catalog: w.catalog,
		DataDir: func(id string) string { return "plugins/" + id + "/data" },
		Changed: func() { w.changed++ },
	})
	return w
}

func (w *pluginDataWorld) diskPath(rel string) string {
	return filepath.Join(w.root, "plugins", testPluginID, "data", filepath.FromSlash(rel))
}

func (w *pluginDataWorld) writeDisk(rel, content string) {
	w.t.Helper()
	p := w.diskPath(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		w.t.Fatal(err)
	}
}

func (w *pluginDataWorld) readDisk(rel string) (string, bool) {
	b, err := os.ReadFile(w.diskPath(rel))
	return string(b), err == nil
}

func digestOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// arrive puts a version in the mirror as a pull from another device would, leaving the disk alone.
func (w *pluginDataWorld) arrive(rel, content string) {
	w.t.Helper()
	err := w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		f := domain.SyncPluginFile{Plugin: testPluginID, Path: rel, Mode: 0o600, SHA256: digestOf(content), Content: []byte(content)}
		d.Sync.PluginFiles = slices.DeleteFunc(d.Sync.PluginFiles, func(x domain.SyncPluginFile) bool { return x.Key() == f.Key() })
		d.Sync.PluginFiles = append(d.Sync.PluginFiles, f)
		return nil
	})
	if err != nil {
		w.t.Fatal(err)
	}
}

func (w *pluginDataWorld) mirror() map[string]string {
	d, _ := w.vault.GetData()
	out := map[string]string{}
	for _, f := range d.Sync.PluginFiles {
		out[f.Path] = string(f.Content)
	}
	return out
}

func (w *pluginDataWorld) reconcile() usecase.PluginDataReport {
	w.t.Helper()
	r, err := w.svc.Reconcile(syncCtx, testPluginID)
	if err != nil {
		w.t.Fatal(err)
	}
	return r
}

func TestPluginDataPushesDeclaredFilesOnly(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	w.writeDisk("notes/a.md", "a1")
	w.writeDisk("tmp/scratch", "x")
	w.writeDisk("cache.db", "x")
	r := w.reconcile()
	if got := w.mirror(); len(got) != 2 || got["state.json"] != "s1" || got["notes/a.md"] != "a1" {
		t.Fatalf("mirror = %v; only declared files, never tmp/ or undeclared ones", got)
	}
	if r.Pushed != 2 || w.changed != 1 || len(r.Skipped) != 0 {
		t.Fatalf("report %+v, changed %d; two files pushed, one sync round asked for, and undeclared files never even read", r, w.changed)
	}
	if r = w.reconcile(); r.Pushed != 0 || w.changed != 1 {
		t.Fatalf("a second pass over unchanged files pushed %d, asked %d rounds", r.Pushed, w.changed)
	}
}

func TestPluginDataAppliesIncomingOnlyWhileStopped(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	w.reconcile()
	w.arrive("state.json", "s2")
	w.arrive("notes/new.md", "n1")

	w.catalog.setRunning(testPluginID, true)
	r := w.reconcile()
	if got, _ := w.readDisk("state.json"); got != "s1" || r.Waiting != 2 {
		t.Fatalf("while running: disk %q, report %+v; nothing is written, two updates wait", got, r)
	}
	if w.svc.Waiting()[testPluginID] != 2 {
		t.Fatalf("waiting = %v; the panel shows two updates waiting", w.svc.Waiting())
	}
	if w.mirror()["state.json"] != "s2" {
		t.Fatal("a running plugin's unchanged file was pushed back over the incoming version")
	}

	w.catalog.setRunning(testPluginID, false)
	r = w.reconcile()
	got, _ := w.readDisk("state.json")
	created, _ := w.readDisk("notes/new.md")
	if got != "s2" || created != "n1" || r.Applied != 2 || w.svc.Waiting()[testPluginID] != 0 {
		t.Fatalf("after stop: disk %q / %q, report %+v, waiting %v", got, created, r, w.svc.Waiting())
	}
	if r = w.reconcile(); r.Pushed != 0 {
		t.Fatalf("the pass after applying pushed %d files back; what it wrote is what is on disk now", r.Pushed)
	}
}

func TestPluginDataStartGuardAppliesBeforeTheProcessStarts(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	w.reconcile()
	w.arrive("state.json", "s2")
	release := w.svc.GuardStart(syncCtx, testPluginID)
	got, _ := w.readDisk("state.json")
	done := make(chan struct{})
	go func() {
		_, _ = w.svc.Reconcile(syncCtx, testPluginID)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a reconcile ran while a start held the guard")
	default:
	}
	release()
	<-done
	if got != "s2" {
		t.Fatalf("disk before start = %q, want the incoming s2", got)
	}
}

func TestPluginDataDeletionsTravelBothWays(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	w.writeDisk("notes/a.md", "a1")
	w.reconcile()
	if err := os.Remove(w.diskPath("notes/a.md")); err != nil {
		t.Fatal(err)
	}
	w.reconcile()
	if _, ok := w.mirror()["notes/a.md"]; ok {
		t.Fatal("a file deleted here stays in the mirror")
	}
	err := w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Sync.PluginFiles = slices.DeleteFunc(d.Sync.PluginFiles, func(f domain.SyncPluginFile) bool { return f.Path == "state.json" })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w.reconcile()
	if _, ok := w.readDisk("state.json"); ok {
		t.Fatal("a file deleted elsewhere stays on disk")
	}
}

func TestPluginDataTwoEditsLastWriterWins(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	w.reconcile()
	w.arrive("state.json", "theirs")
	w.writeDisk("state.json", "mine")
	r := w.reconcile()
	if w.mirror()["state.json"] != "mine" || len(r.Conflicts) != 1 || r.Conflicts[0] != "state.json" {
		t.Fatalf("mirror %q, report %+v; the later local write wins and the conflict is reported", w.mirror()["state.json"], r)
	}
}

func TestPluginDataSkipsOversizeAndUndeclared(t *testing.T) {
	w := newPluginDataWorld(t)
	big := make([]byte, domainplugin.MaxSyncFileBytes+1)
	w.writeDisk("notes/big.bin", string(big))
	w.arrive("notes/big.bin", "theirs")
	w.arrive("secrets.txt", "planted")
	r := w.reconcile()
	if w.mirror()["notes/big.bin"] != "theirs" {
		t.Fatal("an oversize local file replaced or deleted the synced version")
	}
	if _, ok := w.readDisk("secrets.txt"); ok {
		t.Fatal("a file this device's manifest does not declare was written")
	}
	if !slices.Contains(r.Skipped, "notes/big.bin") || !slices.Contains(r.Skipped, "secrets.txt") {
		t.Fatalf("skipped = %v; both are reported", r.Skipped)
	}
}

func TestPluginDataFileCountLimit(t *testing.T) {
	w := newPluginDataWorld(t)
	for i := range domainplugin.MaxSyncPluginFiles + 1 {
		w.writeDisk(fmt.Sprintf("notes/%05d", i), "x")
	}
	r := w.reconcile()
	if len(w.mirror()) != domainplugin.MaxSyncPluginFiles || len(r.Skipped) != 1 {
		t.Fatalf("mirrored %d, skipped %d; the plugin syncs at most %d files", len(w.mirror()), len(r.Skipped), domainplugin.MaxSyncPluginFiles)
	}
}

func TestPluginDataRespectsTheDeviceToggleAndTheAccount(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("state.json", "s1")
	if err := w.svc.SetSyncEnabled(syncCtx, testPluginID, false); err != nil {
		t.Fatal(err)
	}
	if w.reconcile(); len(w.mirror()) != 0 {
		t.Fatal("a plugin turned off on this device synced its data")
	}
	if err := w.svc.SetSyncEnabled(syncCtx, testPluginID, true); err != nil {
		t.Fatal(err)
	}
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Sync.Account = nil; return nil })
	if w.reconcile(); len(w.mirror()) != 0 {
		t.Fatal("plugin data was mirrored into a vault that does not sync")
	}
}

func TestPluginDataANarrowedManifestDeletesNothingElsewhere(t *testing.T) {
	w := newPluginDataWorld(t)
	w.writeDisk("notes/a.md", "a1")
	w.reconcile()
	w.catalog.mu.Lock()
	w.catalog.manifests[0].Sync = &domainplugin.SyncSpec{Paths: []string{"state.json"}}
	w.catalog.mu.Unlock()
	if err := os.Remove(w.diskPath("notes/a.md")); err != nil {
		t.Fatal(err)
	}
	w.reconcile()
	if w.mirror()["notes/a.md"] != "a1" {
		t.Fatal("a path this device no longer declares was pushed as a deletion to every device")
	}
}
