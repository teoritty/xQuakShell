package plugin_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/usecase"
)

// The ui capability (ADR-015) against a real plugin process.
//
// Every layer below this was covered with fakes; what fakes cannot show is whether the layers are
// connected — whether a manifest that declares `ui` on disk lets a real child process open a tab,
// whether its bytes survive the gate and the framing, whether the user's keystrokes reach it, and
// whether closing the session takes its tabs down and tells it so.
//
// This is the automated half of the manual walkthrough the plan calls for. It does not and cannot
// verify what the panels LOOK like; it verifies that everything behind them works.

const (
	uiFixtureID  = "com.xquakshell.fixture-ui-fake"
	uiConnection = "conn-ui-e2e"
	uiSession    = "sess-ui-e2e"
)

type uiRig struct {
	registry *usecase.PluginRegistry
	manager  *usecase.PluginManager
	leader   *usecase.DiscoveryLeader
	service  *usecase.DiscoveryService
	surfaces *usecase.SurfaceService
	dialogs  *usecase.DialogService
	details  *usecase.DiscoveryDetailsService

	presenter *e2ePresenter
	dialogUI  *e2eDialogPresenter
	// dialogAudits is what an incident review would have to work from: the audit trail of answers
	// handed to the plugin (ADR-015 §Security model).
	dialogAudits []domainplugin.DialogAuditEntry
}

// e2ePresenter stands in for the frontend: it records what the user would have seen.
type e2ePresenter struct {
	mu      sync.Mutex
	opened  []domainplugin.Surface
	outputs []string
	states  []string
	closed  []string
}

func (p *e2ePresenter) Opened(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, s)
}

func (p *e2ePresenter) Output(surfaceID, dataBase64, stream string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outputs = append(p.outputs, surfaceID+"|"+stream+"|"+dataBase64)
}

func (p *e2ePresenter) Changed(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states = append(p.states, s.ID+"|"+s.State+"|"+s.Title)
}

func (p *e2ePresenter) Closed(surfaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, surfaceID)
}

func (p *e2ePresenter) snapshot() ([]domainplugin.Surface, []string, []string, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]domainplugin.Surface(nil), p.opened...),
		append([]string(nil), p.outputs...),
		append([]string(nil), p.states...),
		append([]string(nil), p.closed...)
}

type e2eDialogPresenter struct {
	mu     sync.Mutex
	opened []domainplugin.Dialog
	closed []string
}

func (p *e2eDialogPresenter) DialogOpened(d domainplugin.Dialog) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, d)
}

func (p *e2eDialogPresenter) DialogClosed(dialogID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, dialogID)
}

func (p *e2eDialogPresenter) DialogError(string, string, map[string]string) {}

func (p *e2eDialogPresenter) current() (domainplugin.Dialog, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.opened) == 0 {
		return domainplugin.Dialog{}, false
	}
	return p.opened[len(p.opened)-1], true
}

// uiSessions answers the one question the surface service asks about a session.
type uiSessions struct{}

func (uiSessions) ConnectionForSession(sessionID string) (string, bool) {
	if sessionID == uiSession {
		return uiConnection, true
	}
	return "", false
}

func newUIRig(t *testing.T) *uiRig {
	t.Helper()
	pluginDir := buildFixturePlugin(t, "plugin-ui-fake")

	rig := &uiRig{presenter: &e2ePresenter{}, dialogUI: &e2eDialogPresenter{}}
	rig.registry = usecase.NewPluginRegistry()

	authorizer := usecase.NewPluginSessionAuthorizer(rig.registry)
	discoveryHolder := &discoveryInboundHolder{}
	surfaceHolder := &uiSurfaceHolder{}
	dialogHolder := &uiDialogHolder{}
	detailsHolder := &uiDetailsHolder{}

	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot: t.TempDir(),
		SessionRPC: usecase.NewPluginSessionRPCHandlerFactory(
			usecase.NewPluginSessionInbound(), nil, discoveryHolder, surfaceHolder, dialogHolder, detailsHolder, authorizer),
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
	rig.leader = usecase.NewDiscoveryLeader(sshOnlyProtocols{}, rig.registry, rig.manager, store, observer, pace, nil)
	observer.SetLeader(rig.leader)
	rig.service = usecase.NewDiscoveryService(
		store, observer,
		usecase.NewDiscoveryPublishRouter(store, observer, rig.leader, pace, rig.registry),
		usecase.NewDiscoveryInvoker(store, rig.leader, rig.manager, nil),
	)
	discoveryHolder.set(rig.service)

	rig.surfaces = usecase.NewSurfaceService(
		usecase.NewSurfaceRegistry(),
		rig.presenter,
		usecase.NewSurfaceNotifier(rig.manager),
		uiSessions{},
		rig.registry.UICapabilities,
		nil,
	)
	surfaceHolder.port = rig.surfaces

	rig.dialogs = usecase.NewDialogService(
		usecase.NewDialogRegistry(),
		rig.dialogUI,
		usecase.NewDialogNotifier(rig.manager),
		rig.registry.UICapabilities,
		func(entry domainplugin.DialogAuditEntry) { rig.dialogAudits = append(rig.dialogAudits, entry) },
	)
	dialogHolder.port = rig.dialogs

	rig.details = usecase.NewDiscoveryDetailsService(
		store,
		rig.leader,
		rig.manager,
		nil,
		rig.registry.UICapabilities,
		usecase.NewDiscoveryPace(
			usecase.NewDiscoveryPublishLimiter(nil),
			usecase.NewDiscoveryEmitCoalescer(nil, nil, nil),
		),
	)
	detailsHolder.port = rig.details

	rig.manager.SetProcessStartedHandler(observer.PluginStarted)
	rig.manager.SetProcessStoppedHandler(func(pluginID string) {
		rig.service.ClearPlugin(pluginID)
		// The composition root does exactly this, and the reason is worth reproducing here: a
		// stopped plugin's tabs and modals must go with it, or the user is left looking at a stream
		// nobody is writing to.
		rig.surfaces.CloseSurfacesForPlugin(pluginID)
		rig.dialogs.CancelForPlugin(pluginID)
	})
	t.Cleanup(func() { rig.manager.StopAll(context.Background()) })

	discoverer := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := rig.manager.DiscoverPlugins(discoverer.Discover); err != nil {
		t.Fatalf("discover fixture plugin: %v", err)
	}
	return rig
}

// The three holders exist for the same reason the composition root's do: the services are built
// after the RPC factory that must reach them.
type uiSurfaceHolder struct {
	port domainplugin.SurfaceInboundPort
}

func (h *uiSurfaceHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	if h.port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return h.port.Handle(ctx, pluginID, method, params)
}

type uiDialogHolder struct {
	port domainplugin.DialogInboundPort
}

func (h *uiDialogHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	if h.port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return h.port.Handle(ctx, pluginID, method, params)
}

type uiDetailsHolder struct {
	port domainplugin.DiscoveryDetailsInboundPort
}

func (h *uiDetailsHolder) PublishDetails(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	if h.port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return h.port.PublishDetails(ctx, pluginID, params)
}

func (r *uiRig) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// fixtureState asks the plugin what it believes, so assertions cover both ends of the wire rather
// than only the host's.
func (r *uiRig) fixtureState(t *testing.T) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := r.manager.CallWithTimeout(ctx, uiFixtureID, "fixture.state", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("fixture.state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode fixture state: %v", err)
	}
	return state
}

// A log surface, opened by a real plugin over real framing, carrying real bytes.
func TestUIEndToEndLogSurface(t *testing.T) {
	rig := newUIRig(t)
	rig.leader.SessionReady(uiSession, uiConnection)
	rig.service.SetObserved(uiConnection, []string{""})

	rig.waitFor(t, "the fixture to publish its node", func() bool {
		snapshot := rig.service.Snapshot(uiConnection)
		for _, tree := range snapshot.Plugins {
			if len(tree.Nodes) > 0 {
				return true
			}
		}
		return false
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rig.service.InvokeAction(ctx, uiConnection, uiFixtureID, []string{"thing"}, "open-log"); err != nil {
		t.Fatalf("InvokeAction: %v", err)
	}

	rig.waitFor(t, "the surface to open", func() bool {
		opened, _, _, _ := rig.presenter.snapshot()
		return len(opened) == 1
	})
	opened, _, _, _ := rig.presenter.snapshot()
	if opened[0].Kind != domainplugin.SurfaceKindLog {
		t.Fatalf("kind = %q, want log", opened[0].Kind)
	}
	if opened[0].ConnectionID != uiConnection {
		t.Fatalf("connectionId = %q — the host must resolve it, the plugin never sends one", opened[0].ConnectionID)
	}
	if opened[0].Title != "fixture log" {
		t.Fatalf("title = %q", opened[0].Title)
	}

	rig.waitFor(t, "the surface bytes", func() bool {
		_, outputs, _, _ := rig.presenter.snapshot()
		return len(outputs) == 1
	})
	_, outputs, _, _ := rig.presenter.snapshot()
	if !strings.Contains(outputs[0], "|stdout|") {
		t.Fatalf("output = %q", outputs[0])
	}
	rig.waitFor(t, "the ready state", func() bool {
		_, _, states, _ := rig.presenter.snapshot()
		return len(states) > 0
	})
	_, _, states, _ := rig.presenter.snapshot()
	if !strings.Contains(states[len(states)-1], "|ready|") {
		t.Fatalf("states = %v", states)
	}

	// Closing the session takes the tab down and tells the plugin — the lifetime rule, over a real
	// process.
	rig.surfaces.CloseSurfacesForSession(uiSession)
	rig.waitFor(t, "the tab to close", func() bool {
		_, _, _, closed := rig.presenter.snapshot()
		return len(closed) == 1
	})
	rig.waitFor(t, "the plugin to be told", func() bool {
		state := rig.fixtureState(t)
		closed, _ := state["closedSurfaces"].([]any)
		return len(closed) == 1
	})
}

// A terminal surface takes keystrokes and a resize; a log surface takes neither.
func TestUIEndToEndTerminalSurfaceReceivesInput(t *testing.T) {
	rig := newUIRig(t)
	rig.leader.SessionReady(uiSession, uiConnection)
	rig.service.SetObserved(uiConnection, []string{""})
	rig.waitFor(t, "the fixture node", func() bool {
		return len(rig.service.Snapshot(uiConnection).Plugins) > 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rig.service.InvokeAction(ctx, uiConnection, uiFixtureID, []string{"thing"}, "open-terminal"); err != nil {
		t.Fatalf("InvokeAction: %v", err)
	}
	rig.waitFor(t, "the terminal surface", func() bool {
		opened, _, _, _ := rig.presenter.snapshot()
		return len(opened) == 1
	})
	opened, _, _, _ := rig.presenter.snapshot()
	surfaceID := opened[0].ID

	rig.surfaces.DeliverInput(surfaceID, []byte("whoami\r"))
	rig.surfaces.DeliverResize(surfaceID, 132, 43)

	rig.waitFor(t, "the plugin to receive input and a resize", func() bool {
		state := rig.fixtureState(t)
		inputs, _ := state["inputs"].([]any)
		resizes, _ := state["resizes"].([]any)
		return len(inputs) == 1 && len(resizes) == 1
	})
}

// A form dialog, answered by the user, delivering exactly one answer to the plugin.
func TestUIEndToEndDialogRoundTrip(t *testing.T) {
	rig := newUIRig(t)
	rig.leader.SessionReady(uiSession, uiConnection)
	rig.service.SetObserved(uiConnection, []string{""})
	rig.waitFor(t, "the fixture node", func() bool {
		return len(rig.service.Snapshot(uiConnection).Plugins) > 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rig.service.InvokeAction(ctx, uiConnection, uiFixtureID, []string{"thing"}, "open-form"); err != nil {
		t.Fatalf("InvokeAction: %v", err)
	}
	rig.waitFor(t, "the dialog to open", func() bool {
		_, ok := rig.dialogUI.current()
		return ok
	})
	dialog, _ := rig.dialogUI.current()
	if dialog.Kind != domainplugin.DialogKindForm || dialog.CountFields() != 2 {
		t.Fatalf("dialog = %+v", dialog)
	}

	// The user answers. The keyValue field goes through the same validation a real form would.
	if err := rig.dialogs.Submit(dialog.ID, map[string]string{
		"name":   "from-the-user",
		"labels": `{"env":"prod"}`,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	rig.waitFor(t, "the plugin to receive exactly one answer", func() bool {
		state := rig.fixtureState(t)
		if state["lastAnswer"] != "submit" {
			return false
		}
		values, _ := state["answerValues"].(map[string]any)
		return values["name"] == "from-the-user" && values["labels"] == `{"env":"prod"}`
	})

	// Cancelling an already-answered dialog must do nothing: exactly one answer per dialog.
	if err := rig.dialogs.Cancel(dialog.ID); err == nil {
		t.Fatal("a dialog that was already submitted must refuse a second answer")
	}
	state := rig.fixtureState(t)
	if state["lastAnswer"] != "submit" {
		t.Fatalf("the plugin saw a second answer: %v", state["lastAnswer"])
	}
}

// The node details panel: describe, then save, over a real process.
func TestUIEndToEndNodeDetails(t *testing.T) {
	rig := newUIRig(t)
	rig.leader.SessionReady(uiSession, uiConnection)
	rig.service.SetObserved(uiConnection, []string{""})
	rig.waitFor(t, "the fixture node", func() bool {
		return len(rig.service.Snapshot(uiConnection).Plugins) > 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	details, err := rig.details.Describe(ctx, uiConnection, uiFixtureID, "thing")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !details.Editable || len(details.Sections) != 1 || details.Values["shell"] != "/bin/sh" {
		t.Fatalf("details = %+v", details)
	}

	if err := rig.details.Apply(ctx, uiConnection, uiFixtureID, "thing", map[string]string{
		"shell":  "/bin/bash",
		"sneaky": "should be dropped",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	state := rig.fixtureState(t)
	values, _ := state["answerValues"].(map[string]any)
	if values["details.shell"] != "/bin/bash" {
		t.Fatalf("the plugin did not receive the edited value: %v", values)
	}
	if _, leaked := values["details.sneaky"]; leaked {
		t.Fatalf("an undeclared field reached the plugin: %v", values)
	}
}

// A node the host is not currently showing must not be describable: otherwise the details path is
// a way to address anything at all.
func TestUIEndToEndDetailsRefuseAnUnknownNode(t *testing.T) {
	rig := newUIRig(t)
	rig.leader.SessionReady(uiSession, uiConnection)
	rig.service.SetObserved(uiConnection, []string{""})
	rig.waitFor(t, "the fixture node", func() bool {
		return len(rig.service.Snapshot(uiConnection).Plugins) > 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := rig.details.Describe(ctx, uiConnection, uiFixtureID, "no-such-node"); err == nil {
		t.Fatal("expected an unknown node to be refused")
	}
}
