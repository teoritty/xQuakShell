package syncflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infracache "xquakshell/internal/infra/cache"
	infraforge "xquakshell/internal/infra/forge"
	infragithub "xquakshell/internal/infra/github"
	"xquakshell/internal/infra/persistence"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/infra/portable"
	"xquakshell/internal/usecase"
)

const (
	echoID      = "com.example.echo"
	echoRepoURL = "https://github.com/example/echo"
	echoTag     = "v1.0.0"
)

// echoManifest declares synced data, so the same plugin serves S8 (by reference) and S9 (its data).
func echoManifest(description string) string {
	return fmt.Sprintf(`{
  "id": %q,
  "name": "Echo",
  "version": "1.0.0",
  "description": %q,
  "engine": {"type": "go-binary", "entry": "echo.exe"},
  "isolation": "per-plugin",
  "sync": {"paths": ["**"]}
}`, echoID, description)
}

func bundleAsset() string { return "echo-" + runtime.GOOS + "-" + runtime.GOARCH + ".xqsp" }

// packEcho builds a signed-checksum bundle (.xqsp carries SHA256SUMS) from a manifest.
func packEcho(t *testing.T, manifest string) []byte {
	t.Helper()
	src := t.TempDir()
	for name, body := range map[string]string{"plugin.json": manifest, "echo.exe": "stub"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), bundleAsset())
	if err := bundle.Pack(src, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeGitHub is a local GitHub serving one repository, example/echo, with one release whose assets
// the test can replace behind the same tag - the attack S8 must refuse.
type fakeGitHub struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	manifest string
	assets   map[string][]byte
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	g := &fakeGitHub{t: t}
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	g.publish(echoManifest("first build"))
	return g
}

// publish replaces the release behind echoTag with a bundle built from manifest.
func (g *fakeGitHub) publish(manifest string) {
	body := packEcho(g.t, manifest)
	sum := sha256.Sum256(body)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.manifest = manifest
	g.assets = map[string][]byte{
		bundleAsset(): body,
		"SHA256SUMS":  []byte(hex.EncodeToString(sum[:]) + "  " + bundleAsset() + "\n"),
	}
}

func (g *fakeGitHub) release() map[string]any {
	var assets []map[string]any
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range g.assets {
			if !yield(k) {
				return
			}
		}
	}) {
		assets = append(assets, map[string]any{"name": name, "size": len(g.assets[name]),
			"browser_download_url": g.srv.URL + "/download/" + name})
	}
	return map[string]any{"tag_name": echoTag, "name": echoTag, "published_at": "2026-10-01T00:00:00Z", "assets": assets}
}

func (g *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	const repo = "/repos/example/echo"
	switch p := r.URL.Path; {
	case p == repo+"/contents/"+domainplugin.XQSPManifestFile:
		_, _ = w.Write([]byte(g.manifest))
	case p == repo+"/releases/latest" || p == repo+"/releases/tags/"+echoTag:
		_ = json.NewEncoder(w).Encode(g.release())
	case p == repo+"/releases":
		_ = json.NewEncoder(w).Encode([]any{g.release()})
	case strings.HasPrefix(p, "/download/"):
		body, ok := g.assets[strings.TrimPrefix(p, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	default:
		http.NotFound(w, r)
	}
}

// pluginHost stands in for the process host; a test marks a plugin running to hold its data.
type pluginHost struct {
	mu      sync.Mutex
	running map[string]bool
}

func (h *pluginHost) setRunning(id string, on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running[id] = on
}

func (h *pluginHost) Start(_ context.Context, p domainplugin.InstalledPlugin, _ string, _ domainplugin.SandboxPolicy) error {
	h.setRunning(p.Manifest.ID, true)
	return nil
}
func (h *pluginHost) Stop(_ context.Context, id, _ string) error { h.setRunning(id, false); return nil }
func (h *pluginHost) StopAll(context.Context)                    {}
func (h *pluginHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (h *pluginHost) CallWithTimeout(context.Context, string, string, string, json.RawMessage, time.Duration) (json.RawMessage, error) {
	return nil, nil
}
func (h *pluginHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (h *pluginHost) State(id, _ string) domainplugin.ProcessState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running[id] {
		return domainplugin.ProcessRunning
	}
	return domainplugin.ProcessStopped
}
func (h *pluginHost) RunningInstances() []domainplugin.ProcessInstance {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []domainplugin.ProcessInstance
	for id, on := range h.running {
		if on {
			out = append(out, domainplugin.ProcessInstance{PluginID: id, State: domainplugin.ProcessRunning})
		}
	}
	return out
}
func (h *pluginHost) BindSession(string, string) error { return nil }
func (h *pluginHost) UnbindSession(string, string)     {}

// plugins is a device's plugin runtime composed as main_plugin_github.go and main_plugin_data.go
// compose it: the real manager, loader, installer, GitHub pipeline (pointed at the fake GitHub), the
// jailed data zone, plugin data sync and plugins by reference over the device's own vault.
type plugins struct {
	d       *device
	root    string
	host    *pluginHost
	manager *usecase.PluginManager
	repos   *usecase.GitHubRepositoryService
	github  *usecase.GitHubPluginService
	refs    *usecase.PluginReferenceSync
	data    *usecase.PluginDataSync
	mu      sync.Mutex
	reports []usecase.PluginDataReport
}

func (d *device) plugins(g *fakeGitHub) *plugins {
	d.t.Helper()
	p := &plugins{d: d, root: filepath.Join(d.dir, "portable"), host: &pluginHost{running: map[string]bool{}}}
	zone := portable.NewDataStore(p.root, "", nil)
	p.manager = usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{Registry: usecase.NewPluginRegistry(), Host: p.host,
		LoadBundle: infraplugin.LoadPluginSource, InstallBundle: infraplugin.InstallFromSource, InstallRoot: p.root, PortableData: zone})
	storage, err := persistence.NewFileGitHubRepositoryStorage(filepath.Join(d.dir, "github"))
	if err != nil {
		d.t.Fatal(err)
	}
	cache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	router := infraforge.NewRouterWithClients(map[domainplugin.Forge]infraforge.Client{
		domainplugin.ForgeGitHub: infragithub.NewUseCaseClient(infragithub.NewClientWithBaseURL(g.srv.URL))})
	tmp := filepath.Join(d.dir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		d.t.Fatal(err)
	}
	p.repos = usecase.NewGitHubRepositoryService(storage, cache)
	p.github = usecase.NewGitHubPluginService(router, infraplugin.NewBinaryDownloader(router, tmp),
		infraplugin.NewGitHubPluginStager(tmp), infraplugin.InstallMetaWriter{}, cache, p.manager, storage)
	p.data = usecase.NewPluginDataSync(usecase.PluginDataSyncDeps{Zone: zone, Vault: d.vault, Catalog: p.manager,
		DataDir: func(id string) string {
			rel, _ := filepath.Rel(p.root, infraplugin.PluginDataDir(p.root, id))
			return filepath.ToSlash(rel)
		},
		Reported: func(r usecase.PluginDataReport) { p.mu.Lock(); p.reports = append(p.reports, r); p.mu.Unlock() },
	})
	p.manager.SetStartGuard(p.data.GuardStart)
	p.refs = usecase.NewPluginReferenceSync(usecase.PluginReferenceDeps{Vault: d.vault, Installed: p.manager.Registry().List,
		Installer: p.github,
		Disable:   func(ctx context.Context, id string) error { return p.manager.SetPluginEnabled(ctx, id, false) },
		Enable:    func(ctx context.Context, id string) error { return p.manager.SetPluginEnabled(ctx, id, true) },
	})
	// As in production: an install or uninstall here records the plugin at once.
	p.manager.SetInstalledChangedHandler(func() {
		if err := p.refs.Record(ctx); err != nil {
			d.t.Errorf("record plugins: %v", err)
		}
	})
	return p
}

func (p *plugins) installed() (domainplugin.InstalledPlugin, bool) {
	got, err := p.manager.Registry().Get(echoID)
	return got, err == nil
}

func (p *plugins) offer() (usecase.PluginOffer, bool) {
	p.d.t.Helper()
	list, err := p.refs.Offers(ctx)
	if err != nil {
		p.d.t.Fatal(err)
	}
	for _, o := range list {
		if o.Plugin.ID == echoID {
			return o, true
		}
	}
	return usecase.PluginOffer{}, false
}

// S8: a plugin installed from GitHub on A is offered on B, never installed by itself; installing the
// offer runs the real pipeline against the fake GitHub and verifies the digests; a release replaced
// behind the same tag is refused on the next device, with nothing written.
func TestS8PluginByReference(t *testing.T) {
	s := newServer(t)
	g := newFakeGitHub(t)
	a := s.registered()
	ap := a.plugins(g)
	if err := ap.repos.AddRepository(ctx, echoRepoURL, false); err != nil {
		t.Fatal(err)
	}
	if err := ap.github.InstallPluginFromGitHub(ctx, echoRepoURL, echoTag, false, false, false, false, false, false); err != nil {
		t.Fatalf("install on A: %v", err)
	}
	onA, _ := ap.installed()
	a.sync()

	b := s.enrolled("phone", "correct horse")
	bp := b.plugins(g)
	o, ok := bp.offer()
	if !ok || o.State != usecase.PluginOfferInstall || o.Plugin.ManifestDigest != onA.ManifestDigest {
		t.Fatalf("offer on B = %+v (%v); A's plugin is offered by reference with A's digests (G: spec 6.4)", o, ok)
	}
	if _, ok := bp.installed(); ok {
		t.Fatal("B installed the plugin by itself; an offer never installs")
	}
	if err := bp.refs.InstallOffer(ctx, usecase.OfferInstallRequest{PluginID: echoID}); err != nil {
		t.Fatalf("install the offer on B: %v", err)
	}
	onB, ok := bp.installed()
	if !ok || onB.ManifestDigest != onA.ManifestDigest || onB.ChecksumsDigest != onA.ChecksumsDigest {
		t.Fatalf("B installed %+v; the verified build is exactly A's", onB)
	}
	if _, ok := bp.offer(); ok {
		t.Fatal("the plugin is still offered on B after installing it")
	}
	b.sync()
	a.sync()

	// T3': a replaced release behind the same tag.
	g.publish(echoManifest("replaced behind the same tag"))
	c := s.enrolled("tablet", "correct horse")
	cp := c.plugins(g)
	err := cp.refs.InstallOffer(ctx, usecase.OfferInstallRequest{PluginID: echoID})
	if !errors.Is(err, usecase.ErrPluginDigestMismatch) {
		t.Fatalf("install of a replaced release on C: %v, want ErrPluginDigestMismatch (T3': digest check, spec 6.4)", err)
	}
	if _, ok := cp.installed(); ok {
		t.Fatal("C registered the replaced release")
	}
	if _, err := os.Stat(filepath.Join(cp.root, "plugins", echoID)); !os.IsNotExist(err) {
		t.Fatalf("the replaced release left files on C: %v", err)
	}
	if o, _ := cp.offer(); o.State != usecase.PluginOfferInstall {
		t.Fatalf("C's offer after the refusal = %+v; it stays offered until the user decides", o)
	}
}

// installLocal installs echo from a bundle file, as the Plugins screen does for a local install.
func (p *plugins) installLocal() {
	p.d.t.Helper()
	path := filepath.Join(p.d.t.TempDir(), bundleAsset())
	if err := os.WriteFile(path, packEcho(p.d.t, echoManifest("local")), 0o600); err != nil {
		p.d.t.Fatal(err)
	}
	if _, err := p.manager.Install(path, domainplugin.InstallTrustPolicy{}, false, false, false); err != nil {
		p.d.t.Fatalf("install: %v", err)
	}
}

func (p *plugins) dataPath(rel string) string {
	return filepath.Join(infraplugin.PluginDataDir(p.root, echoID), filepath.FromSlash(rel))
}

func (p *plugins) write(rel, content string) {
	p.d.t.Helper()
	path := p.dataPath(rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		p.d.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		p.d.t.Fatal(err)
	}
}

func (p *plugins) read(rel string) (string, bool) {
	b, err := os.ReadFile(p.dataPath(rel))
	return string(b), err == nil
}

func (p *plugins) reconcile() usecase.PluginDataReport {
	p.d.t.Helper()
	r, err := p.data.Reconcile(ctx, echoID)
	if err != nil {
		p.d.t.Fatalf("reconcile: %v", err)
	}
	return r
}

// maxVersions is the most versions the server retains for any one record of d's vault.
func (s *server) maxVersions(d *device) int {
	s.t.Helper()
	stored, err := d.state.LoadVault(ctx, d.session.Personal)
	if err != nil {
		s.t.Fatal(err)
	}
	p := s.h.Principal(s.tokenOf(d))
	most := 0
	for _, e := range stored.Records {
		vs, err := s.h.Svc.Versions(ctx, p, d.session.Personal, e.Record)
		if err != nil {
			s.t.Fatal(err)
		}
		most = max(most, len(vs))
	}
	return most
}

// S9: the files a plugin declares follow the user; tmp/ never does; incoming files wait while the
// plugin runs and land when it stops; two edits of one file resolve last-writer-wins with the
// replaced version kept in the vault's history and the user told.
func TestS9PluginData(t *testing.T) {
	s := newServer(t)
	g := newFakeGitHub(t)
	a := s.registered()
	ap := a.plugins(g)
	ap.installLocal()
	b := s.enrolled("phone", "correct horse")
	bp := b.plugins(g)
	bp.installLocal()

	ap.write("state.json", "v1")
	ap.write("notes/a.md", "note")
	ap.write("tmp/scratch", "never synced")
	if r := ap.reconcile(); r.Pushed != 2 {
		t.Fatalf("A pushed %d files, want two: tmp/ is never synced, even when the manifest declares everything (spec 6.5)", r.Pushed)
	}
	a.sync()
	b.sync()

	bp.host.setRunning(echoID, true)
	if r := bp.reconcile(); r.Waiting != 2 || r.Applied != 0 {
		t.Fatalf("B while running: %+v; incoming files wait for the plugin to stop", r)
	}
	if _, ok := bp.read("state.json"); ok {
		t.Fatal("a file was written under a running plugin")
	}
	bp.host.setRunning(echoID, false)
	if r := bp.reconcile(); r.Applied != 2 {
		t.Fatalf("B after stop: %+v; both files land", r)
	}
	if got, _ := bp.read("state.json"); got != "v1" {
		t.Fatalf("B's state.json = %q", got)
	}
	if _, ok := bp.read("tmp/scratch"); ok {
		t.Fatal("tmp/ reached another device")
	}

	// Last writer wins: A edits and syncs first; B edits later, pulls A's edit, then scans.
	ap.write("state.json", "from A")
	ap.reconcile()
	a.sync()
	bp.write("state.json", "from B")
	b.sync()
	r := bp.reconcile()
	if !slices.Equal(r.Conflicts, []string{"state.json"}) {
		t.Fatalf("B's report %+v; the user is told about the overwritten edit", r)
	}
	b.sync()
	a.sync()
	ap.reconcile()
	gotA, _ := ap.read("state.json")
	gotB, _ := bp.read("state.json")
	if gotA != "from B" || gotB != "from B" {
		t.Fatalf("A has %q, B has %q; both converge on the last writer's version", gotA, gotB)
	}
	if n := s.maxVersions(a); n < 3 {
		t.Fatalf("the server keeps at most %d versions of a record; the replaced edit must stay in history for the trash window", n)
	}
}
