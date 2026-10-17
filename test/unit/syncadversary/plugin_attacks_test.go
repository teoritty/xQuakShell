package syncadversary

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/portable"
	"xquakshell/internal/usecase"
)

const victimPlugin = "com.example.notes"

func hostileFile(path, content string, mode uint32) domain.SyncPluginFile {
	sum := sha256.Sum256([]byte(content))
	return domain.SyncPluginFile{Plugin: victimPlugin, Path: path, Mode: mode, SHA256: hex.EncodeToString(sum[:]), Content: []byte(content)}
}

// Malicious device, plugin records (spec 12.5, T3'): B's engine pushes whatever B's vault holds, and
// B's vault holds plugin data and plugin references no honest client would write. Every one is
// quarantined on A - nothing reaches A's vault, so nothing can reach A's disk or install pipeline.
func TestMaliciousDevicePluginRecords(t *testing.T) {
	_, a, b := pair(t)
	badDigest := hostileFile("state.json", "x", 0o600)
	badDigest.SHA256 = strings.Repeat("00", 32)
	files := []domain.SyncPluginFile{
		hostileFile("../vault.age", "x", 0o600),
		hostileFile("/etc/passwd", "x", 0o600),
		hostileFile("C:/Windows/win.ini", "x", 0o600),
		hostileFile("notes/a.md:hidden", "x", 0o600),
		hostileFile("notes/../../outside", "x", 0o600),
		hostileFile(`notes\..\outside`, "x", 0o600),
		hostileFile("tmp/state", "x", 0o600),
		hostileFile("run.sh", "#!/bin/sh", 0o755),
		badDigest,
		{Plugin: "../evil", Path: "state.json", Mode: 0o600, SHA256: hostileFile("", "x", 0).SHA256, Content: []byte("x")},
	}
	digest := strings.Repeat("ab", 32)
	plugins := []domain.SyncPlugin{
		{ID: "com.example.a", Version: "1", ManifestDigest: digest, SumsDigest: digest,
			Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "https://evil.example/x", Tag: "v1"}},
		{ID: "com.example.b", Version: "1", ManifestDigest: digest, SumsDigest: digest,
			Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/b/../../x", Tag: "v1"}},
		{ID: "com.example.c", Version: "1", ManifestDigest: "not a digest",
			Source: domain.SyncPluginSource{Kind: domain.PluginSourceManual}},
		{ID: "com.example.d", Version: "1", ManifestDigest: digest,
			Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/d", Tag: "v1"}},
	}
	_ = b.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		if v.Sync == nil {
			v.Sync = &domain.VaultSync{}
		}
		v.Sync.PluginFiles = append(v.Sync.PluginFiles, files...)
		v.Sync.Plugins = append(v.Sync.Plugins, plugins...)
		return nil
	})
	b.sync()

	r, err := a.engine.Sync(ctx, a.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(files) + len(plugins); r.Pull.Quarantined != want {
		t.Fatalf("A quarantined %d records, want all %d hostile ones", r.Pull.Quarantined, want)
	}
	got, _ := a.vault.GetData()
	if got.Sync != nil && (len(got.Sync.PluginFiles) != 0 || len(got.Sync.Plugins) != 0) {
		t.Fatalf("hostile plugin records reached A's vault: files %+v, plugins %+v", got.Sync.PluginFiles, got.Sync.Plugins)
	}
}

// dataCatalog is A's installed plugin: it declares every path, so only the never-synced rules and
// the zone stand between an incoming file and the disk.
type dataCatalog struct{}

func (*dataCatalog) SyncedManifests() []domainplugin.Manifest {
	return []domainplugin.Manifest{{ID: victimPlugin, Name: "Notes", Sync: &domainplugin.SyncSpec{Paths: []string{"**"}}}}
}
func (*dataCatalog) ProcessIdle(string) bool { return true }

// Malicious device, a valid record aimed at a link: B pushes notes/x.md, which passes every record
// rule, while A's data directory holds a link named notes that points outside it. The zone refuses
// to write through the link; the file outside is never touched.
func TestMaliciousDevicePluginFileThroughALink(t *testing.T) {
	_, a, b := pair(t)
	root := t.TempDir()
	outside := t.TempDir()
	dataDir := filepath.Join(root, "plugins", victimPlugin, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	plantDirLink(t, outside, filepath.Join(dataDir, "notes"))

	_ = b.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		if v.Sync == nil {
			v.Sync = &domain.VaultSync{}
		}
		v.Sync.PluginFiles = append(v.Sync.PluginFiles, hostileFile("notes/x.md", "planted", 0o600))
		return nil
	})
	b.sync()
	a.sync()
	if got, _ := a.vault.GetData(); got.Sync == nil || len(got.Sync.PluginFiles) != 1 {
		t.Fatal("the valid record did not reach A's vault mirror; the test would not reach the zone")
	}

	data := usecase.NewPluginDataSync(usecase.PluginDataSyncDeps{Zone: portable.NewDataStore(root, "", nil), Vault: a.vault,
		Catalog: &dataCatalog{}, DataDir: func(id string) string { return "plugins/" + id + "/data" }})
	r, err := data.Reconcile(ctx, victimPlugin)
	if err != nil || r.Applied != 0 || !slices.Equal(r.Skipped, []string{"notes/x.md"}) {
		t.Fatalf("reconcile = %+v, %v; the file behind a link is skipped and reported, never written", r, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := slices.Collect(func(yield func(string) bool) {
			for _, e := range entries {
				if !yield(e.Name()) {
					return
				}
			}
		})
		t.Fatalf("files were written outside the data root through a link: %v", names)
	}
}
