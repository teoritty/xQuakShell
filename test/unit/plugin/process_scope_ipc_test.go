package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

type scopedNotifyHost struct {
	mu       sync.Mutex
	notifies []scopedNotifyCall
	state    domainplugin.ProcessState
}

type scopedNotifyCall struct {
	pluginID  string
	sessionID string
	method    string
}

func (h *scopedNotifyHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	h.state = domainplugin.ProcessRunning
	return nil
}

func (h *scopedNotifyHost) Stop(context.Context, string, string) error { return nil }

func (h *scopedNotifyHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (h *scopedNotifyHost) Notify(_ context.Context, pluginID, sessionID, method string, _ json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifies = append(h.notifies, scopedNotifyCall{pluginID: pluginID, sessionID: sessionID, method: method})
	return nil
}

func (h *scopedNotifyHost) State(string, string) domainplugin.ProcessState { return h.state }

func (h *scopedNotifyHost) StopAll(context.Context) {}

func (h *scopedNotifyHost) RunningInstances() []domainplugin.ProcessInstance {
	return []domainplugin.ProcessInstance{{
		PluginID:  "com.test.per-session",
		SessionID: "sess-99",
		State:     h.state,
	}}
}

func (h *scopedNotifyHost) BindSession(string, string) error { return nil }

func (h *scopedNotifyHost) UnbindSession(string, string) {}

func (h *scopedNotifyHost) lastNotify() (scopedNotifyCall, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notifies) == 0 {
		return scopedNotifyCall{}, false
	}
	return h.notifies[len(h.notifies)-1], true
}

func TestEventBusDeliversCoreSessionEventWithSessionScope(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	const pluginID = "com.test.per-session"
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:        pluginID,
			Name:      "Per Session",
			Version:   "1.0.0",
			Isolation: domainplugin.IsolationPerSession,
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{Subscribe: []string{"core.session.*"}},
			},
		},
		RootDir: t.TempDir(),
	})

	host := &scopedNotifyHost{state: domainplugin.ProcessRunning}
	manager := newTestPluginManager(t, registry, host)
	bus := usecase.NewPluginEventBus(registry, manager.NotifyProcess)

	if err := bus.Subscribe(context.Background(), pluginID, "core.session.*"); err != nil {
		t.Fatal(err)
	}
	bus.SetSessionActiveChecker(func(string) bool { return true })
	bus.SetSessionOwnershipChecker(sessionOwnerStub{owns: map[string]bool{
		pluginID + ":sess-99": true,
	}})

	bus.PublishCore(context.Background(), "core.session.opened", map[string]string{
		"sessionId": "sess-99",
	})

	call, ok := host.lastNotify()
	if !ok {
		t.Fatal("expected notify to per-session process")
	}
	if call.pluginID != pluginID || call.sessionID != "sess-99" || call.method != "event" {
		t.Fatalf("unexpected notify: %+v", call)
	}
}

func TestEnsureRunningRejectsPerSessionIsolation(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:        "com.test.per-session",
			Name:      "Per Session",
			Version:   "1.0.0",
			Isolation: domainplugin.IsolationPerSession,
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		},
		RootDir: t.TempDir(),
	})
	manager := newTestPluginManager(t, registry, &scopedNotifyHost{})

	err := manager.EnsureRunning(context.Background(), "com.test.per-session")
	if !errors.Is(err, domainplugin.ErrSessionScopeRequired) {
		t.Fatalf("expected ErrSessionScopeRequired, got %v", err)
	}
}

func TestStartPluginManualRejectsPerSessionIsolation(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:        "com.test.per-session",
			Name:      "Per Session",
			Version:   "1.0.0",
			Isolation: domainplugin.IsolationPerSession,
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			ActivationEvents: []string{
				"onManual",
			},
		},
		RootDir: t.TempDir(),
	})
	manager := newTestPluginManager(t, registry, &scopedNotifyHost{})

	err := manager.StartPluginManual(context.Background(), "com.test.per-session")
	if !errors.Is(err, domainplugin.ErrSessionScopeRequired) {
		t.Fatalf("expected ErrSessionScopeRequired, got %v", err)
	}
}

func TestAggregateProcessStateFindsPerSessionInstance(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	const pluginID = "com.test.per-session"
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:        pluginID,
			Name:      "Per Session",
			Version:   "1.0.0",
			Isolation: domainplugin.IsolationPerSession,
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		},
		RootDir: t.TempDir(),
	})
	host := &scopedNotifyHost{state: domainplugin.ProcessRunning}
	manager := newTestPluginManager(t, registry, host)

	list := manager.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(list))
	}
	if list[0].State != string(domainplugin.ProcessRunning) {
		t.Fatalf("expected running state in list, got %q", list[0].State)
	}
}

func TestStopPluginStopsAllSessionInstances(t *testing.T) {
	host := &multiInstanceHost{
		instances: []domainplugin.ProcessInstance{
			{PluginID: "com.test.multi", SessionID: "s1", State: domainplugin.ProcessRunning},
			{PluginID: "com.test.multi", SessionID: "s2", State: domainplugin.ProcessRunning},
		},
	}
	registry := usecase.NewPluginRegistry()
	manager := newTestPluginManager(t, registry, host)

	if err := manager.StopPlugin(context.Background(), "com.test.multi"); err != nil {
		t.Fatal(err)
	}
	if host.stopCount != 2 {
		t.Fatalf("expected 2 stops, got %d", host.stopCount)
	}
}

func TestViewRelayTokenConsumedOnRelay(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.views", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Contributions: domainplugin.Contributions{
				Views: []domainplugin.ViewContribution{{
					ID: "panel.one", Location: "sidebar.bottom", Title: "One",
				}},
			},
		},
		RootDir: t.TempDir(),
	})

	manager := newTestPluginManager(t, registry, &scopedNotifyHost{})
	relay := usecase.NewPluginViewRelay(manager, registry)

	token, err := relay.PreparePanel("com.test.views", "panel.one")
	if err != nil {
		t.Fatal(err)
	}

	_ = relay.RelayMessage(context.Background(), token, json.RawMessage(`{"x":1}`))

	err = relay.RelayMessage(context.Background(), token, json.RawMessage(`{"x":2}`))
	if !errors.Is(err, domainplugin.ErrViewRelayTokenInvalid) {
		t.Fatalf("expected token consumed, got %v", err)
	}
}

func TestManifestRejectsViewsWithPerSessionIsolation(t *testing.T) {
	m := domainplugin.Manifest{
		ID:        "com.test.bad",
		Name:      "Bad",
		Version:   "1.0.0",
		Isolation: domainplugin.IsolationPerSession,
		Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			Views: []domainplugin.ViewContribution{{
				ID: "panel", Location: "sidebar.bottom", Title: "Panel",
			}},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected views with per-session isolation to be rejected")
	}
}

type multiInstanceHost struct {
	instances []domainplugin.ProcessInstance
	stopCount int
}

func (h *multiInstanceHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	return nil
}

func (h *multiInstanceHost) Stop(context.Context, string, string) error {
	h.stopCount++
	return nil
}

func (h *multiInstanceHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}

func (h *multiInstanceHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}

func (h *multiInstanceHost) State(string, string) domainplugin.ProcessState {
	return domainplugin.ProcessDiscovered
}

func (h *multiInstanceHost) StopAll(context.Context) {}

func (h *multiInstanceHost) RunningInstances() []domainplugin.ProcessInstance {
	return h.instances
}

func (h *multiInstanceHost) BindSession(string, string) error { return nil }

func (h *multiInstanceHost) UnbindSession(string, string) {}
