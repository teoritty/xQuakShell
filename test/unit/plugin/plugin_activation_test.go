package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/usecase"
)

func TestExecuteEchoCommand(t *testing.T) {
	pluginDir := buildExampleEchoPlugin(t)

	inbound := usecase.NewPluginSessionInbound()
	var manager *usecase.PluginManager
	registry := usecase.NewPluginRegistry()
	eventBus := usecase.NewPluginEventBus(registry, func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
		if manager == nil {
			return nil
		}
		return manager.NotifyProcess(ctx, pluginID, sessionID, method, params)
	})

	auth := usecase.NewPluginSessionAuthorizer(registry)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, auth),
		Events:            eventBus,
		SessionAuthorizer: auth,
	})
	manager = newTestPluginManager(t, registry, host)
	manager.SetEventBus(eventBus)

	discovery := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := manager.DiscoverPlugins(discovery.Discover); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const pluginID = "com.xquakshell.example-echo"
	raw, err := manager.ExecuteCommand(ctx, pluginID, "echo.ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["message"] != "Echo ping OK" {
		t.Fatalf("unexpected result: %v", result)
	}

	manager.StopAll(ctx)
}

func TestEventBusDeliverCoreEvent(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.events",
		Name:    "Events Test",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "test.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Subscribe: []string{"core.session.*"},
			},
		},
	}
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: manifest,
		RootDir:  t.TempDir(),
		Source:   domainplugin.SourceBundled,
	})

	received := make(chan string, 1)
	bus := usecase.NewPluginEventBus(registry, func(_ context.Context, pluginID, _ string, method string, _ json.RawMessage) error {
		if pluginID == manifest.ID && method == "event" {
			received <- pluginID
		}
		return nil
	})

	if err := bus.Subscribe(context.Background(), manifest.ID, "core.session.stateChanged"); err != nil {
		t.Fatal(err)
	}

	bus.PublishCore(context.Background(), "core.session.stateChanged", map[string]string{
		"sessionId": "s1",
		"state":     "ready",
	})

	select {
	case id := <-received:
		if id != manifest.ID {
			t.Fatalf("unexpected plugin id %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not delivered")
	}
}

func TestMatchesActivation(t *testing.T) {
	events := []string{"onStartup", "onProtocol:telnet", "onCommand:echo.ping"}

	if !usecase.MatchesActivation(events, usecase.ActivationTrigger{Kind: usecase.ActivationStartup}) {
		t.Fatal("expected startup match")
	}
	if !usecase.MatchesActivation(events, usecase.ActivationTrigger{Kind: usecase.ActivationProtocol, Value: "telnet"}) {
		t.Fatal("expected protocol match")
	}
	if !usecase.MatchesActivation(events, usecase.ActivationTrigger{Kind: usecase.ActivationCommand, Value: "echo.ping"}) {
		t.Fatal("expected command match")
	}
	if usecase.MatchesActivation(events, usecase.ActivationTrigger{Kind: usecase.ActivationProtocol, Value: "ssh"}) {
		t.Fatal("unexpected protocol match")
	}
}

func TestChannelPatternMatching(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.pub",
		Name:    "Pub Test",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "test.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Publish: []string{"plugin.com.test.pub.*"},
			},
		},
	}
	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: manifest, RootDir: t.TempDir()})

	bus := usecase.NewPluginEventBus(registry, func(context.Context, string, string, string, json.RawMessage) error {
		return nil
	})

	payload, _ := json.Marshal(map[string]string{"ok": "1"})
	err := bus.PublishFromPlugin(context.Background(), manifest.ID, "plugin.com.test.pub.custom", payload)
	if err != nil {
		t.Fatal(err)
	}

	err = bus.PublishFromPlugin(context.Background(), manifest.ID, "plugin.other.custom", payload)
	if err == nil {
		t.Fatal("expected publish denied for foreign namespace")
	}
}

func TestExecuteCommandRequiresActivationOrRunning(t *testing.T) {
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot: t.TempDir(),
	})
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test.nostart",
			Name:    "No Start",
			Version: "1.0.0",
			Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "missing.exe"},
			Contributions: domainplugin.Contributions{
				Commands: []domainplugin.CommandContribution{{ID: "cmd.test", Title: "Test"}},
			},
		},
		RootDir: t.TempDir(),
	})
	manager := newTestPluginManager(t, registry, host)

	ctx := context.Background()
	_, err := manager.ExecuteCommand(ctx, "com.test.nostart", "cmd.test", nil)
	if !errors.Is(err, domainplugin.ErrPluginNotRunning) {
		t.Fatalf("expected not running without activation, got %v", err)
	}
}

func TestEventPublishRateLimited(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.ratelimit",
		Name:    "Rate",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "test.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Publish: []string{"plugin.com.test.ratelimit.*"},
			},
		},
	}
	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: manifest, RootDir: t.TempDir()})

	bus := usecase.NewPluginEventBus(registry, func(context.Context, string, string, string, json.RawMessage) error {
		return nil
	})

	payload, _ := json.Marshal(map[string]string{"ok": "1"})
	channel := "plugin.com.test.ratelimit.custom"
	for i := 0; i < 101; i++ {
		if err := bus.PublishFromPlugin(context.Background(), manifest.ID, channel, payload); err != nil {
			if errors.Is(err, domainplugin.ErrRateLimited) {
				return
			}
			t.Fatalf("unexpected error at %d: %v", i, err)
		}
	}
	t.Fatal("expected rate limit error")
}
