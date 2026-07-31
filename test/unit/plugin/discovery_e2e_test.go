package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/usecase"
)

// This is the only test in the suite that runs the discovery path end to end against a real plugin
// process. Everything below it was already covered layer by layer with fakes; what could not be
// covered that way is whether the layers are actually connected — whether a manifest capability
// declared on disk causes a real child process to be addressed, whether its snapshot survives the
// gate, the IDOR check and the store, and whether the icon a group declared reaches an instance
// that declared none.
//
// The fixture binary is built by the test itself (buildDiscoveryFixturePlugin). A skip when the
// binary is missing was deliberately not written: a test that switches itself off reports green for
// the one condition under which it proves nothing.

const (
	discoveryFixtureID  = "com.xquakshell.fixture-discovery-fake"
	discoveryConnection = "conn-e2e"
	discoverySession    = "sess-e2e"
)

func TestDiscoveryEndToEndWithLivePlugin(t *testing.T) {
	rig := newDiscoveryRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. The plugin is installed, declared capabilities.discovery, and is therefore addressable for
	//    an ssh connection. A plugin absent from this list is never sent an observe at all.
	assertDiscoveryTarget(t, rig.registry)

	if err := rig.manager.EnsureRunning(ctx, discoveryFixtureID); err != nil {
		t.Fatalf("start fixture plugin: %v", err)
	}
	t.Cleanup(func() { rig.manager.StopAll(context.Background()) })

	// The binding is what the IDOR check consults when discovery.publish names a session
	// (ADR-014 §Security model). Without it every snapshot below would be refused.
	if err := rig.manager.BindSession(discoveryFixtureID, discoverySession); err != nil {
		t.Fatalf("bind session: %v", err)
	}
	rig.leader.SessionReady(discoverySession, discoveryConnection)

	// 2. Observing the connection root makes the plugin publish exactly one group.
	rig.service.SetObserved(discoveryConnection, []string{""})
	rig.waitForNodes(t, "fake")
	if ids := rig.nodeIDs(); len(ids) != 1 {
		t.Fatalf("observing the root alone must yield only the root group, got %v", ids)
	}

	// 3. Observing the group as well yields its two subgroups, and nothing deeper: the instances
	//    below them are not observed, so a correct plugin has not enumerated them.
	rig.service.SetObserved(discoveryConnection, []string{"", "fake"})
	rig.waitForNodes(t, "fake", "alpha", "beta")
	if ids := rig.nodeIDs(); len(ids) != 3 {
		t.Fatalf("collapsed subgroups must not be enumerated, got %v", ids)
	}

	// 4. Expanding one subgroup yields its instances, and the icon they resolve to is the
	//    subgroup's own — not the root group's. The instances declare no icon at all, so this is
	//    inheritance measured on live data rather than on a hand-built node.
	rig.service.SetObserved(discoveryConnection, []string{"", "fake", "alpha"})
	rig.waitForNodes(t, "fake", "alpha", "beta", "alpha-1", "alpha-2", "alpha-3", "alpha-4")

	assertIcon(t, rig.view(t, "fake"), "root")
	assertIcon(t, rig.view(t, "alpha"), "left")
	assertIcon(t, rig.view(t, "beta"), "right")
	for _, id := range []string{"alpha-1", "alpha-2", "alpha-3", "alpha-4"} {
		view := rig.view(t, id)
		if view.Node.IconID != "" {
			t.Fatalf("instance %q must declare no icon of its own, got %q", id, view.Node.IconID)
		}
		assertIcon(t, view, "left")
	}

	// A reported neutral tone and no status at all must stay distinguishable all the way through
	// the wire, the store and the read model.
	assertTone(t, rig.view(t, "alpha-1"), "ok")
	assertTone(t, rig.view(t, "alpha-2"), "warn")
	assertTone(t, rig.view(t, "alpha-3"), "error")
	if rig.view(t, "alpha-4").Node.Status != nil {
		t.Fatal("a node the plugin reported no status for must arrive with no status")
	}

	// 5. An action reaches the plugin, is acknowledged, and its result arrives as an ordinary
	//    publish that changes the node's tone.
	if err := rig.service.InvokeAction(ctx, discoveryConnection, discoveryFixtureID, []string{"alpha-2"}, "refresh"); err != nil {
		t.Fatalf("invoke refresh: %v", err)
	}
	rig.waitFor(t, "alpha-2 reports ok after the action", func() bool {
		view, ok := rig.lookup("alpha-2")
		return ok && view.Node.Status != nil && view.Node.Status.Tone == discovery.ToneOK
	})
	assertAudited(t, rig.auditLines(), "refresh", "alpha-2")

	// A mass action over siblings takes the same verb with a longer list; a single-node action
	// asked to run over two is refused by the host before the plugin hears about it.
	if err := rig.service.InvokeAction(ctx, discoveryConnection, discoveryFixtureID, []string{"alpha-1", "alpha-3"}, "refresh"); err != nil {
		t.Fatalf("invoke mass refresh: %v", err)
	}
	rig.waitFor(t, "both mass-action targets report ok", func() bool {
		one, okOne := rig.lookup("alpha-1")
		three, okThree := rig.lookup("alpha-3")
		return okOne && okThree &&
			one.Node.Status != nil && one.Node.Status.Tone == discovery.ToneOK &&
			three.Node.Status != nil && three.Node.Status.Tone == discovery.ToneOK
	})
	if err := rig.service.InvokeAction(ctx, discoveryConnection, discoveryFixtureID, []string{"alpha-1", "alpha-3"}, "inspect"); !errors.Is(err, usecase.ErrDiscoveryActionUnavailable) {
		t.Fatalf("a non-multi action over two nodes must be refused by the host, got %v", err)
	}

	// 6. Collapsing a subgroup takes it out of the observed set, and the plugin stops publishing
	//    for it. The assertion is made on the plugin's own counters: from the host's side a
	//    dropped publish and an unsent one look identical, and it is the unsent one the
	//    level-triggered protocol promises.
	before := rig.pluginStats(ctx, t)
	rig.service.SetObserved(discoveryConnection, []string{"", "fake", "beta"})
	rig.waitForNodes(t, "fake", "alpha", "beta", "alpha-1", "alpha-2", "alpha-3", "alpha-4",
		"beta-1", "beta-2", "beta-3")
	after := rig.pluginStats(ctx, t)

	if after.Published["beta"] <= before.Published["beta"] {
		t.Fatalf("the newly expanded subgroup must have been published: %v -> %v", before.Published, after.Published)
	}
	if after.Published["alpha"] != before.Published["alpha"] {
		t.Fatalf("a collapsed subgroup must not be published for any more: %v -> %v", before.Published, after.Published)
	}
	if containsString(after.Observed, "alpha") {
		t.Fatalf("the collapsed node must be gone from the plugin's observed set, got %v", after.Observed)
	}

	// 7. The session that carried all of it closes with no replacement, and the tree is deleted
	//    rather than cached: nothing is left that could confirm the resources still exist.
	rig.leader.SessionClosed(discoverySession, discoveryConnection)
	if snapshot := rig.service.Snapshot(discoveryConnection); len(snapshot.Plugins) != 0 {
		t.Fatalf("closing the last ready session must delete the tree, got %d plugin trees", len(snapshot.Plugins))
	}
}

// TestDiscoveryFixtureIconsReachTheFrontendAsDataURIs covers the other half of the icon contract:
// the IDs resolved above only mean something if the bytes behind them travel to the frontend on the
// existing plugin list, base64-encoded, with no path and no icon endpoint (ADR-014 §manifest).
func TestDiscoveryFixtureIconsReachTheFrontendAsDataURIs(t *testing.T) {
	rig := newDiscoveryRig(t)

	var icons map[string]string
	for _, info := range rig.manager.List() {
		if info.ID == discoveryFixtureID {
			icons = info.DiscoveryIcons
		}
	}
	if len(icons) != 3 {
		t.Fatalf("expected three declared icons on the plugin list, got %v", icons)
	}
	for _, id := range []string{"root", "left", "right"} {
		uri, ok := icons[id]
		if !ok {
			t.Fatalf("icon %q missing from the plugin list", id)
		}
		if !strings.HasPrefix(uri, "data:image/svg+xml;base64,") {
			t.Fatalf("icon %q must travel as a base64 data URI, got %.40q", id, uri)
		}
	}
	if icons["root"] == icons["left"] || icons["left"] == icons["right"] {
		t.Fatal("each declared icon must carry its own bytes; identical URIs would hide an inheritance bug")
	}
}

// discoveryRig is the production wiring of main_plugins.go, reduced to what discovery needs and
// pointed at a temporary install root. It is assembled in the same order and with the same
// late-binding holder, because the cycle it breaks (service needs manager needs host needs the
// session RPC factory needs the service) is a property of the design, not of the app.
type discoveryRig struct {
	registry *usecase.PluginRegistry
	manager  *usecase.PluginManager
	leader   *usecase.DiscoveryLeader
	service  *usecase.DiscoveryService

	mu    sync.Mutex
	audit []string
}

func newDiscoveryRig(t *testing.T) *discoveryRig {
	t.Helper()
	pluginDir := buildDiscoveryFixturePlugin(t)

	rig := &discoveryRig{}
	rig.registry = usecase.NewPluginRegistry()
	rig.registry.SetDiscoveryIconAssetReader(infrapluginassets.DiscoveryIconReader{})

	authorizer := usecase.NewPluginSessionAuthorizer(rig.registry)
	inboundHolder := &discoveryInboundHolder{}

	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(usecase.NewPluginSessionInbound(), nil, inboundHolder, authorizer),
		SessionAuthorizer: authorizer,
	})
	rig.manager = usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    rig.registry,
		Host:        host,
		InstallRoot: t.TempDir(),
	})

	store := usecase.NewDiscoveryStore()
	observer := usecase.NewDiscoveryObserver(rig.registry, rig.manager)
	pace := usecase.NewDiscoveryPace(
		usecase.NewDiscoveryPublishLimiter(nil),
		usecase.NewDiscoveryEmitCoalescer(func(string, string) {}, nil, nil),
	)
	rig.leader = usecase.NewDiscoveryLeader(sshOnlyProtocols{}, store, observer, pace, nil)
	observer.SetLeader(rig.leader)
	rig.service = usecase.NewDiscoveryService(
		store,
		observer,
		usecase.NewDiscoveryPublishRouter(store, observer, rig.leader, pace, rig.registry),
		usecase.NewDiscoveryInvoker(store, rig.leader, rig.manager, rig.recordAudit),
	)
	inboundHolder.set(rig.service)
	rig.manager.SetProcessStartedHandler(observer.PluginStarted)
	rig.manager.SetProcessStoppedHandler(rig.service.ClearPlugin)

	discoverer := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := rig.manager.DiscoverPlugins(discoverer.Discover); err != nil {
		t.Fatalf("discover fixture plugin: %v", err)
	}
	return rig
}

func (r *discoveryRig) recordAudit(entry domainplugin.DiscoveryAuditEntry) {
	r.mu.Lock()
	r.audit = append(r.audit, entry.Action+" "+entry.ActionID+" "+strings.Join(entry.NodeIDs, ","))
	r.mu.Unlock()
}

func (r *discoveryRig) auditLines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.audit...)
}

// discoveryInboundHolder mirrors the composition root's holder: the port is resolved when a plugin
// calls, not when the host is built.
type discoveryInboundHolder struct {
	mu   sync.RWMutex
	port domainplugin.DiscoveryInboundPort
}

func (h *discoveryInboundHolder) set(port domainplugin.DiscoveryInboundPort) {
	h.mu.Lock()
	h.port = port
	h.mu.Unlock()
}

func (h *discoveryInboundHolder) Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.RLock()
	port := h.port
	h.mu.RUnlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Publish(ctx, pluginID, params)
}

// sshOnlyProtocols stands in for the session registry. The leader asks it one question — what
// protocol a session speaks — and the answer decides whether a plugin declaring parentProtocols
// ["ssh"] is addressed at all.
type sshOnlyProtocols struct{}

func (sshOnlyProtocols) ProtocolForSession(string) (string, bool) { return "ssh", true }

// waitFor polls until cond holds. Polling rather than sleeping a fixed interval: every step here
// crosses a process boundary in both directions, and a fixed sleep would be either flaky or slow.
func (r *discoveryRig) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting until %s; tree now holds %v", what, r.nodeIDs())
}

func (r *discoveryRig) waitForNodes(t *testing.T, want ...string) {
	t.Helper()
	r.waitFor(t, "the tree holds "+strings.Join(want, ", "), func() bool {
		for _, id := range want {
			if _, ok := r.lookup(id); !ok {
				return false
			}
		}
		return true
	})
}

func (r *discoveryRig) lookup(nodeID string) (usecase.DiscoveryNodeView, bool) {
	for _, tree := range r.service.Snapshot(discoveryConnection).Plugins {
		for _, view := range tree.Nodes {
			if view.Node.ID == nodeID {
				return view, true
			}
		}
	}
	return usecase.DiscoveryNodeView{}, false
}

func (r *discoveryRig) view(t *testing.T, nodeID string) usecase.DiscoveryNodeView {
	t.Helper()
	view, ok := r.lookup(nodeID)
	if !ok {
		t.Fatalf("node %q is not in the tree; it holds %v", nodeID, r.nodeIDs())
	}
	return view
}

func (r *discoveryRig) nodeIDs() []string {
	var ids []string
	for _, tree := range r.service.Snapshot(discoveryConnection).Plugins {
		for _, view := range tree.Nodes {
			ids = append(ids, view.Node.ID)
		}
	}
	return ids
}

// fixtureStats is the fixture's self-report: how many snapshots it sent per branch, and what it
// currently believes it was asked to watch.
type fixtureStats struct {
	Published map[string]int `json:"published"`
	Observed  []string       `json:"observed"`
}

func (r *discoveryRig) pluginStats(ctx context.Context, t *testing.T) fixtureStats {
	t.Helper()
	raw, err := r.manager.Call(ctx, discoveryFixtureID, "fake.stats", nil)
	if err != nil {
		t.Fatalf("ask the fixture for its stats: %v", err)
	}
	var stats fixtureStats
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("decode fixture stats: %v", err)
	}
	return stats
}

func assertDiscoveryTarget(t *testing.T, registry *usecase.PluginRegistry) {
	t.Helper()
	for _, target := range registry.DiscoveryPlugins() {
		if target.PluginID != discoveryFixtureID {
			continue
		}
		if !containsString(target.ParentProtocols, "ssh") {
			t.Fatalf("fixture must declare ssh as a parent protocol, got %v", target.ParentProtocols)
		}
		return
	}
	t.Fatal("the installed fixture did not reach the registry as a discovery target")
}

func assertIcon(t *testing.T, view usecase.DiscoveryNodeView, want string) {
	t.Helper()
	if view.Icon != want {
		t.Fatalf("node %q resolved icon %q, want %q", view.Node.ID, view.Icon, want)
	}
}

func assertTone(t *testing.T, view usecase.DiscoveryNodeView, want discovery.Tone) {
	t.Helper()
	if view.Node.Status == nil || view.Node.Status.Tone != want {
		t.Fatalf("node %q: want tone %q, got %+v", view.Node.ID, want, view.Node.Status)
	}
}

func assertAudited(t *testing.T, lines []string, actionID, nodeID string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, actionID) && strings.Contains(line, nodeID) {
			return
		}
	}
	t.Fatalf("no audit entry naming action %q on node %q, got %v", actionID, nodeID, lines)
}

// buildDiscoveryFixturePlugin compiles the fixture and lays out an installable bundle, icons
// included. It is a variant of buildFixturePlugin rather than a parameter on it: this is the only
// fixture with assets, and threading a copy-these-directories argument through the shared helper
// would complicate every caller for one.
func buildDiscoveryFixturePlugin(t *testing.T) string {
	t.Helper()
	installDir := buildFixturePlugin(t, "plugin-discovery-fake")
	src := filepath.Join(repoRoot(t), "test", "fixtures", "plugin-discovery-fake", "ui", "icons")
	dst := filepath.Join(installDir, "ui", "icons")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("fixture icons missing: %v", err)
	}
	for _, entry := range entries {
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), readFile(t, filepath.Join(src, entry.Name())), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The checksums must be rewritten after the icons land: the bundle validator enforces
	// set-equality both ways, so a file on disk that SHA256SUMS does not list refuses the plugin.
	if err := bundle.WriteChecksums(installDir); err != nil {
		t.Fatal(err)
	}
	return installDir
}
