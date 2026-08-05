package plugin_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/usecase"
)

// The ui capability end to end: the real gate, the real session RPC handler and the real surface
// service, wired the way the composition root wires them.
//
// Everything below the transport is genuine — what is faked is only the two ends nothing here is
// testing: the frontend the presenter talks to, and the plugin process the notifier talks to.
// Spawning a real child process would test the IPC framing, which conn_test.go already covers.

type contractPresenter struct {
	mu      sync.Mutex
	opened  []domainplugin.Surface
	outputs []string
	closed  []string
}

func (p *contractPresenter) Opened(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, s)
}
func (p *contractPresenter) Output(surfaceID, dataBase64, stream string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outputs = append(p.outputs, surfaceID+":"+stream)
}

// waitForOutputs waits for the surface's output pump to deliver. Writes are queued and flushed on
// an interval now, so reading the slice straight after a write would be racing the pump.
func (p *contractPresenter) waitForOutputs(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		got := append([]string(nil), p.outputs...)
		p.mu.Unlock()
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d output batches, got %v", want, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (p *contractPresenter) Changed(domainplugin.Surface) {}
func (p *contractPresenter) Closed(surfaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, surfaceID)
}

type contractOutbound struct {
	mu      sync.Mutex
	inputs  []string
	resizes []string
	closed  []string
}

func (o *contractOutbound) Input(pluginID, surfaceID string, data []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inputs = append(o.inputs, surfaceID+":"+string(data))
}
func (o *contractOutbound) Resize(pluginID, surfaceID string, cols, rows uint16) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.resizes = append(o.resizes, surfaceID)
}
func (o *contractOutbound) Closed(pluginID, surfaceID, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = append(o.closed, surfaceID+":"+reason)
}

type contractSessions map[string]string

func (c contractSessions) ConnectionForSession(sessionID string) (string, bool) {
	id, ok := c[sessionID]
	return id, ok
}

// contractAuthorizer accepts exactly one (plugin, session) pair, so the IDOR path is exercised by
// asking about a different one.
type contractAuthorizer struct {
	pluginID  string
	sessionID string
}

// The binding half is not exercised here: this rig hands the authorizer a fixed pair, and what is
// under test is the refusal, not how a binding is established.
func (a contractAuthorizer) BindSession(pluginID, sessionID string) error { return nil }

func (a contractAuthorizer) UnbindSession(pluginID, sessionID string) {}

func (a contractAuthorizer) AuthorizeSessionRPC(pluginID, processSessionID string, isolation domainplugin.IsolationMode, allowMulti bool, targetSessionID string) error {
	if pluginID == a.pluginID && targetSessionID == a.sessionID {
		return nil
	}
	return domainplugin.ErrSessionNotBound
}

func uiContractManifest(ui *domainplugin.UICaps) domainplugin.Manifest {
	m := domainplugin.Manifest{ID: "com.test.ui", Name: "UI", Version: "1.0.0"}
	m.Engine.Type = "go-binary"
	m.Engine.Entry = "ui.exe"
	m.Capabilities.UI = ui
	return m
}

type uiContractRig struct {
	handler   domainplugin.SessionRPCHandler
	gate      *capability.Gate
	presenter *contractPresenter
	outbound  *contractOutbound
	surfaces  *usecase.SurfaceService
}

func newUIContractRig(t *testing.T, ui *domainplugin.UICaps) *uiContractRig {
	t.Helper()
	manifest := uiContractManifest(ui)
	if err := manifest.ValidateCapabilities(); err != nil {
		t.Fatalf("manifest must be valid for the contract to mean anything: %v", err)
	}
	nd, _, err := domainplugin.Negotiate(&manifest, domainplugin.HostRegistry())
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}

	rig := &uiContractRig{
		gate:      capability.NewGate(manifest, nd),
		presenter: &contractPresenter{},
		outbound:  &contractOutbound{},
	}
	rig.surfaces = usecase.NewSurfaceService(
		usecase.NewSurfaceRegistry(),
		rig.presenter,
		rig.outbound,
		contractSessions{"sess-1": "conn-1"},
		func(string) *domainplugin.UICaps { return ui },
		nil,
	)
	rig.handler = usecase.NewPluginSessionRPCHandler(
		usecase.PluginSessionRPCPorts{
			Sessions: usecase.NewPluginSessionInbound(),
			Surfaces: rig.surfaces,
		},
		contractAuthorizer{pluginID: "com.test.ui", sessionID: "sess-1"},
		usecase.PluginSessionScope{
			PluginID:         "com.test.ui",
			ProcessSessionID: "sess-1",
			Isolation:        domainplugin.IsolationPerPlugin,
		},
	)
	return rig
}

// call runs one plugin->host RPC the way the runtime does: gate first, then the handler.
func (r *uiContractRig) call(t *testing.T, method, params string) (json.RawMessage, error) {
	t.Helper()
	if !r.gate.Allow(method) {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return r.handler.Handle(context.Background(), "com.test.ui", method, json.RawMessage(params))
}

func bothSurfaceKinds() *domainplugin.UICaps {
	return &domainplugin.UICaps{Surfaces: []string{"terminal", "log"}}
}

// The whole lifecycle a plugin sees, in the order it sees it.
func TestUISurfaceContractOpenWriteInputClose(t *testing.T) {
	rig := newUIContractRig(t, bothSurfaceKinds())

	raw, err := rig.call(t, "surface.open", `{"parentSessionId":"sess-1","kind":"terminal","title":"logs"}`)
	if err != nil {
		t.Fatalf("surface.open: %v", err)
	}
	var opened struct {
		SurfaceID string `json:"surfaceId"`
	}
	if err := json.Unmarshal(raw, &opened); err != nil || opened.SurfaceID == "" {
		t.Fatalf("surface.open result = %s (%v)", raw, err)
	}
	if len(rig.presenter.opened) != 1 || rig.presenter.opened[0].ConnectionID != "conn-1" {
		t.Fatalf("the frontend was told %v", rig.presenter.opened)
	}

	payload := base64.StdEncoding.EncodeToString([]byte("hello"))
	if _, err := rig.call(t, "surface.write", `{"surfaceId":"`+opened.SurfaceID+`","dataBase64":"`+payload+`","stream":"stderr"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}
	rig.presenter.waitForOutputs(t, 1)

	// Host -> plugin: what the user types reaches the owner.
	rig.surfaces.DeliverInput(opened.SurfaceID, []byte("ls\r"))
	rig.surfaces.DeliverResize(opened.SurfaceID, 120, 40)
	if len(rig.outbound.inputs) != 1 || len(rig.outbound.resizes) != 1 {
		t.Fatalf("inputs=%v resizes=%v", rig.outbound.inputs, rig.outbound.resizes)
	}

	// The parent session closes: the tab goes, and the plugin is told once.
	rig.surfaces.CloseSurfacesForSession("sess-1")
	if len(rig.presenter.closed) != 1 || len(rig.outbound.closed) != 1 {
		t.Fatalf("presenter=%v plugin=%v", rig.presenter.closed, rig.outbound.closed)
	}

	// Anything the plugin had already queued is dropped rather than erroring: an ordinary race.
	if _, err := rig.call(t, "surface.write", `{"surfaceId":"`+opened.SurfaceID+`","dataBase64":"`+payload+`"}`); err != nil {
		t.Fatalf("a write after teardown must be a no-op, got %v", err)
	}
	// Given a fair chance to flush, nothing more arrives: the queue went with the surface.
	time.Sleep(200 * time.Millisecond)
	if got := rig.presenter.waitForOutputs(t, 1); len(got) != 1 {
		t.Fatalf("a write after teardown must not reach the frontend: %v", got)
	}
}

// A log surface takes no input: the notifier is never addressed for one.
func TestUISurfaceContractLogSurfaceTakesNoInput(t *testing.T) {
	rig := newUIContractRig(t, bothSurfaceKinds())
	raw, err := rig.call(t, "surface.open", `{"parentSessionId":"sess-1","kind":"log","title":"logs"}`)
	if err != nil {
		t.Fatalf("surface.open: %v", err)
	}
	var opened struct {
		SurfaceID string `json:"surfaceId"`
	}
	_ = json.Unmarshal(raw, &opened)

	rig.surfaces.DeliverInput(opened.SurfaceID, []byte("ls\r"))
	rig.surfaces.DeliverResize(opened.SurfaceID, 80, 24)
	if len(rig.outbound.inputs) != 0 || len(rig.outbound.resizes) != 0 {
		t.Fatalf("a log surface must receive neither input nor resize: %v %v",
			rig.outbound.inputs, rig.outbound.resizes)
	}
}

func TestUISurfaceContractGateDeniesWithoutTheCapability(t *testing.T) {
	rig := newUIContractRig(t, &domainplugin.UICaps{Dialogs: true})
	_, err := rig.call(t, "surface.open", `{"parentSessionId":"sess-1","kind":"log","title":"x"}`)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
}

// The IDOR rule, on the same path channel.open and discovery.publish take.
func TestUISurfaceContractRefusesAnUnownedSession(t *testing.T) {
	rig := newUIContractRig(t, bothSurfaceKinds())
	_, err := rig.call(t, "surface.open", `{"parentSessionId":"sess-someone-else","kind":"log","title":"x"}`)
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("got %v, want ErrSessionNotBound", err)
	}
	if len(rig.presenter.opened) != 0 {
		t.Fatal("a refused open must not reach the frontend")
	}
}

// An undeclared kind is refused by the use case, not the gate: the gate never sees a request body.
func TestUISurfaceContractRefusesAnUndeclaredKind(t *testing.T) {
	rig := newUIContractRig(t, &domainplugin.UICaps{Surfaces: []string{"log"}})
	_, err := rig.call(t, "surface.open", `{"parentSessionId":"sess-1","kind":"terminal","title":"x"}`)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
}
