package usecase

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// discoveryCallLog records the ORDER of calls across several fakes. Most tests care only about
// what happened; the ones about the bind/observe race care about which happened first, and that is
// not visible in any single fake's own record.
type discoveryCallLog struct {
	mu     sync.Mutex
	events []string
}

func (l *discoveryCallLog) record(event string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.events = append(l.events, event)
	l.mu.Unlock()
}

func (l *discoveryCallLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// firstIndexOf returns the position of the first event with the given prefix, or -1.
func (l *discoveryCallLog) firstIndexOf(prefix string) int {
	for i, event := range l.all() {
		if strings.HasPrefix(event, prefix) {
			return i
		}
	}
	return -1
}

// fakeDiscoveryNotifier stands in for the plugin process host on the host->plugin notification
// path. Tests assert on what was actually put on the wire, not on whether a method exists.
type fakeDiscoveryNotifier struct {
	mu    sync.Mutex
	sent  []sentObserve
	fails bool
	log   *discoveryCallLog
}

type sentObserve struct {
	pluginID  string
	method    string
	sessionID string
	nodeIDs   []string
}

func (f *fakeDiscoveryNotifier) Notify(_ context.Context, pluginID, method string, params json.RawMessage) error {
	var payload discoveryObservePayload
	_ = json.Unmarshal(params, &payload)
	f.mu.Lock()
	f.sent = append(f.sent, sentObserve{pluginID: pluginID, method: method, sessionID: payload.SessionID, nodeIDs: payload.NodeIDs})
	f.mu.Unlock()
	f.log.record("observe:" + pluginID)
	if f.fails {
		return context.Canceled
	}
	return nil
}

func (f *fakeDiscoveryNotifier) reset() {
	f.mu.Lock()
	f.sent = nil
	f.mu.Unlock()
}

func (f *fakeDiscoveryNotifier) all() []sentObserve {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentObserve(nil), f.sent...)
}

func (f *fakeDiscoveryNotifier) toPlugin(pluginID string) []sentObserve {
	var out []sentObserve
	for _, sent := range f.all() {
		if sent.pluginID == pluginID {
			out = append(out, sent)
		}
	}
	return out
}

// fakeDiscoveryCaller stands in for the plugin process host on the host->plugin request path. Its
// only job in most tests is to prove it was NOT called.
type fakeDiscoveryCaller struct {
	mu    sync.Mutex
	calls []recordedInvoke
	err   error
}

// recordedInvoke keeps the addressee alongside the payload: which plugin a destructive action
// reached is the thing worth asserting, and the payload alone cannot say.
type recordedInvoke struct {
	pluginID string
	payload  discoveryInvokePayload
}

func (f *fakeDiscoveryCaller) CallWithTimeout(_ context.Context, pluginID, _ string, params json.RawMessage, _ time.Duration) (json.RawMessage, error) {
	var payload discoveryInvokePayload
	_ = json.Unmarshal(params, &payload)
	f.mu.Lock()
	f.calls = append(f.calls, recordedInvoke{pluginID: pluginID, payload: payload})
	f.mu.Unlock()
	return nil, f.err
}

func (f *fakeDiscoveryCaller) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// firstCall returns the first recorded call under the fake's own lock. Tests must not reach into
// f.calls directly: the field is mutex-guarded, and a test that reads it bare is a template for the
// next test that adds a goroutine and gets a false pass under -race.
func (f *fakeDiscoveryCaller) firstCall(t *testing.T) recordedInvoke {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("no plugin call was recorded")
	}
	return f.calls[0]
}

type fakeDiscoveryPlugins struct {
	targets []DiscoveryPluginTarget
}

func (f *fakeDiscoveryPlugins) DiscoveryPlugins() []DiscoveryPluginTarget { return f.targets }

// fakeDiscoveryRuntime stands in for the plugin manager on the lifecycle path: which processes the
// leader started, and which (plugin, session) authorizations it granted and released. Those grants
// are what the IDOR check consults, so a test that asserts on them is asserting on the difference
// between a subtree that renders and one that is refused on every publish.
type fakeDiscoveryRuntime struct {
	mu       sync.Mutex
	started  []string
	bound    []discoveryBinding
	unbound  []discoveryBinding
	startErr error
	bindErr  error
	log      *discoveryCallLog
	// onStart stands in for the host's process-started handler, which the real PluginManager fires
	// from inside EnsureRunning — synchronously, before EnsureRunning returns. Without it a test
	// cannot reproduce the ordering that matters: the started plugin is told what to observe while
	// the reconciliation that started it has not finished authorizing it.
	onStart func(pluginID string)
}

func (f *fakeDiscoveryRuntime) EnsureRunning(_ context.Context, pluginID string) error {
	f.mu.Lock()
	f.started = append(f.started, pluginID)
	onStart, err := f.onStart, f.startErr
	f.mu.Unlock()
	f.log.record("start:" + pluginID)
	if err != nil {
		return err
	}
	if onStart != nil {
		onStart(pluginID)
	}
	return nil
}

func (f *fakeDiscoveryRuntime) BindSession(pluginID, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bindErr != nil {
		return f.bindErr
	}
	f.bound = append(f.bound, discoveryBinding{pluginID: pluginID, sessionID: sessionID})
	f.log.record("bind:" + pluginID)
	return nil
}

func (f *fakeDiscoveryRuntime) UnbindSession(pluginID, sessionID string) {
	f.mu.Lock()
	f.unbound = append(f.unbound, discoveryBinding{pluginID: pluginID, sessionID: sessionID})
	f.mu.Unlock()
	f.log.record("unbind:" + pluginID)
}

// live returns the bindings granted and not since released, which is the only question worth
// asking: a plugin bound, unbound and bound again is correctly authorized, and a test comparing
// raw call logs would call that a failure.
func (f *fakeDiscoveryRuntime) live() []discoveryBinding {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []discoveryBinding
	for _, b := range f.bound {
		released := 0
		for _, u := range f.unbound {
			if u == b {
				released++
			}
		}
		granted := 0
		for _, g := range f.bound {
			if g == b {
				granted++
			}
		}
		if granted > released && !containsBinding(out, b) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].pluginID != out[j].pluginID {
			return out[i].pluginID < out[j].pluginID
		}
		return out[i].sessionID < out[j].sessionID
	})
	return out
}

func (f *fakeDiscoveryRuntime) startedPlugins() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func (f *fakeDiscoveryRuntime) wasUnbound(want discoveryBinding) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return containsBinding(f.unbound, want)
}

// fakeDiscoveryIcons stands in for the plugin registry's manifest view: which icon IDs each plugin
// actually declared in contributions.discoveryIcons.
type fakeDiscoveryIcons struct {
	mu       sync.Mutex
	byPlugin map[string][]string
}

func (f *fakeDiscoveryIcons) declare(pluginID string, iconIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byPlugin[pluginID] = iconIDs
}

func (f *fakeDiscoveryIcons) DeclaredDiscoveryIcons(pluginID string) map[string]struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make(map[string]struct{}, len(f.byPlugin[pluginID]))
	for _, id := range f.byPlugin[pluginID] {
		ids[id] = struct{}{}
	}
	return ids
}

// fakeDiscoveryAudit captures the durable record of invokeAction, which is the thing ADR-014 makes
// promises about — not the slog line beside it.
type fakeDiscoveryAudit struct {
	mu      sync.Mutex
	entries []domainplugin.DiscoveryAuditEntry
}

func (f *fakeDiscoveryAudit) record(entry domainplugin.DiscoveryAuditEntry) {
	f.mu.Lock()
	f.entries = append(f.entries, entry)
	f.mu.Unlock()
}

func (f *fakeDiscoveryAudit) all() []domainplugin.DiscoveryAuditEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domainplugin.DiscoveryAuditEntry(nil), f.entries...)
}

type fakeSessionProtocols struct {
	bySession map[string]string
}

func (f *fakeSessionProtocols) ProtocolForSession(sessionID string) (string, bool) {
	protocol, ok := f.bySession[sessionID]
	return protocol, ok
}

// fakeClock is the injected time source. Nothing in these tests sleeps: a rate-limit test that
// sleeps is a rate-limit test that guesses.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// discoveryHarness assembles the whole usecase stack over fakes.
type discoveryHarness struct {
	store     *DiscoveryStore
	observer  *DiscoveryObserver
	leader    *DiscoveryLeader
	service   *DiscoveryService
	pace      *DiscoveryPace
	notifier  *fakeDiscoveryNotifier
	caller    *fakeDiscoveryCaller
	plugins   *fakeDiscoveryPlugins
	runtime   *fakeDiscoveryRuntime
	protocols *fakeSessionProtocols
	icons     *fakeDiscoveryIcons
	audit     *fakeDiscoveryAudit
	clock     *fakeClock
	callLog   *discoveryCallLog

	mu     sync.Mutex
	emits  []emittedChange
	timers []func()
}

// emittedChange is what the frontend would receive: DiscoveryTreeChanged{connectionId, nodeId}.
type emittedChange struct {
	connectionID string
	nodeID       string
}

func newDiscoveryHarness(t *testing.T, targets ...DiscoveryPluginTarget) *discoveryHarness {
	t.Helper()
	if len(targets) == 0 {
		targets = []DiscoveryPluginTarget{{PluginID: "p1", ParentProtocols: []string{"ssh"}}}
	}
	callLog := &discoveryCallLog{}
	h := &discoveryHarness{
		callLog:   callLog,
		notifier:  &fakeDiscoveryNotifier{log: callLog},
		caller:    &fakeDiscoveryCaller{},
		plugins:   &fakeDiscoveryPlugins{targets: targets},
		runtime:   &fakeDiscoveryRuntime{log: callLog},
		protocols: &fakeSessionProtocols{bySession: map[string]string{}},
		icons:     &fakeDiscoveryIcons{byPlugin: map[string][]string{}},
		audit:     &fakeDiscoveryAudit{},
		clock:     newFakeClock(),
	}
	h.store = NewDiscoveryStore()
	h.observer = NewDiscoveryObserver(h.plugins, h.notifier)
	h.pace = NewDiscoveryPace(
		NewDiscoveryPublishLimiter(h.clock.Now),
		NewDiscoveryEmitCoalescer(h.recordEmit, h.clock.Now, h.schedule),
	)
	h.leader = NewDiscoveryLeader(h.protocols, h.plugins, h.runtime, h.store, h.observer, h.pace, h.recordEmit)
	h.observer.SetLeader(h.leader)
	h.service = NewDiscoveryService(
		h.store,
		h.observer,
		NewDiscoveryPublishRouter(h.store, h.observer, h.leader, h.pace, h.icons),
		NewDiscoveryInvoker(h.store, h.leader, h.caller, h.audit.record),
	)
	return h
}

// eventually polls until cond holds. Binding reconciliation runs on its own goroutine — starting a
// plugin process must not block the path that reports a session as ready — so the tests that assert
// on it wait rather than assume.
func (h *discoveryHarness) eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting until %s; bindings now %+v", what, h.runtime.live())
}

func (h *discoveryHarness) recordEmit(connectionID, nodeID string) {
	h.mu.Lock()
	h.emits = append(h.emits, emittedChange{connectionID: connectionID, nodeID: nodeID})
	h.mu.Unlock()
}

func (h *discoveryHarness) lastEmit(t *testing.T) emittedChange {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.emits) == 0 {
		t.Fatal("no change was emitted")
	}
	return h.emits[len(h.emits)-1]
}

func (h *discoveryHarness) schedule(_ time.Duration, fn func()) {
	h.mu.Lock()
	h.timers = append(h.timers, fn)
	h.mu.Unlock()
}

// fireTimers runs every trailing-emit callback the coalescer booked, standing in for wall clock
// expiry.
func (h *discoveryHarness) fireTimers() {
	h.mu.Lock()
	pending := h.timers
	h.timers = nil
	h.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

func (h *discoveryHarness) emitCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.emits)
}

// sessionReady registers a ready ssh session and clears the observe that registering it emitted,
// so a test only sees the notifications its own subject produced.
func (h *discoveryHarness) sessionReady(sessionID, connectionID string) {
	h.protocols.bySession[sessionID] = "ssh"
	h.leader.SessionReady(sessionID, connectionID)
	// The observe for a new leader now goes out at the END of the binding reconciliation, on its own
	// goroutine. Waiting for it here rather than resetting immediately is what keeps every test below
	// seeing only the notifications its own subject produced.
	h.leader.awaitReconcile()
	h.notifier.reset()
}

// publishAs routes a snapshot from a named plugin, for the tests that need more than one publisher.
func (h *discoveryHarness) publishAs(t *testing.T, pluginID, sessionID, nodeID string, children ...discovery.Node) {
	t.Helper()
	if err := h.publish(t, pluginID, sessionID, nodeID, children...); err != nil {
		t.Fatalf("publish %q as %q: %v", nodeID, pluginID, err)
	}
}

func (h *discoveryHarness) publish(t *testing.T, pluginID, sessionID, nodeID string, children ...discovery.Node) error {
	t.Helper()
	return h.service.applyPublish(context.Background(), pluginID, DiscoveryPublish{
		SessionID: sessionID,
		NodeID:    nodeID,
		State:     discovery.BranchReady,
		Children:  children,
	})
}

// nodeIDsOf flattens a snapshot's first plugin tree into the node IDs it contains.
func nodeIDsOf(snapshot DiscoverySnapshot) []string {
	var ids []string
	for _, tree := range snapshot.Plugins {
		for _, view := range tree.Nodes {
			ids = append(ids, view.Node.ID)
		}
	}
	return ids
}

func branchOf(t *testing.T, snapshot DiscoverySnapshot, pluginID, nodeID string) discovery.Branch {
	t.Helper()
	for _, tree := range snapshot.Plugins {
		if tree.PluginID != pluginID {
			continue
		}
		branch, ok := tree.Branches[nodeID]
		if !ok {
			t.Fatalf("no branch for node %q in plugin %q", nodeID, pluginID)
		}
		return branch
	}
	t.Fatalf("no tree for plugin %q", pluginID)
	return discovery.Branch{}
}

func groupNode(id, parentID string, actions ...discovery.Action) discovery.Node {
	return discovery.Node{ID: id, ParentID: parentID, Kind: discovery.KindGroup, Label: id, Actions: actions}
}

func instanceNode(id, parentID string, actions ...discovery.Action) discovery.Node {
	return discovery.Node{ID: id, ParentID: parentID, Kind: discovery.KindInstance, Label: id, Actions: actions}
}

func containsID(ids []string, want string) bool {
	return slices.Contains(ids, want)
}
