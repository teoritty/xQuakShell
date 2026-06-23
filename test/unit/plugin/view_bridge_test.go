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

func TestViewMessageRoundTrip(t *testing.T) {
	pluginDir := buildExampleEchoPlugin(t)

	sessionInbound := usecase.NewPluginSessionInbound()
	registry := usecase.NewPluginRegistry()
	viewInbound := usecase.NewPluginViewInbound(registry)
	received := make(chan json.RawMessage, 1)
	viewInbound.SetHandler(&viewCapture{ch: received})

	var manager *usecase.PluginManager
	eventBus := usecase.NewPluginEventBus(registry, func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
		if manager == nil {
			return nil
		}
		return manager.NotifyProcess(ctx, pluginID, sessionID, method, params)
	})

	auth := usecase.NewPluginSessionAuthorizer(registry)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(sessionInbound, auth),
		Events:            eventBus,
		Views:             viewInbound,
		SessionAuthorizer: auth,
	})
	manager = newTestPluginManager(t, registry, host)

	discovery := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := manager.DiscoverPlugins(discovery.Discover); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const (
		pluginID = "com.xquakshell.example-echo"
		panelID  = "echo.panel"
	)

	if err := manager.EnsureRunning(ctx, pluginID); err != nil {
		t.Fatal(err)
	}

	msg, _ := json.Marshal(map[string]string{"action": "ping"})
	if err := manager.PostViewMessage(ctx, pluginID, panelID, msg); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-received:
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["echo"] != "pong from plugin" {
			t.Fatalf("unexpected payload: %v", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("plugin view reply not received")
	}

	manager.StopAll(ctx)
}

func TestViewPostMessageIDOR(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test.views",
			Name:    "Views",
			Version: "1.0.0",
			Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Contributions: domainplugin.Contributions{
				Views: []domainplugin.ViewContribution{{
					ID: "owned.panel", Location: "sidebar.bottom", Title: "Owned",
				}},
			},
		},
		RootDir: t.TempDir(),
	})

	viewInbound := usecase.NewPluginViewInbound(registry)
	msg, _ := json.Marshal(map[string]string{"x": "1"})
	err := viewInbound.PostMessage(context.Background(), "com.test.views", "foreign.panel", msg)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected capability denied, got %v", err)
	}
}

type viewCapture struct {
	ch chan json.RawMessage
}

func (v *viewCapture) HandlePluginViewMessage(_ string, _ string, message json.RawMessage) error {
	v.ch <- message
	return nil
}

func TestMergeContributionsIncludesViews(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "p1",
			Name:    "P",
			Version: "1",
			Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Contributions: domainplugin.Contributions{
				Views: []domainplugin.ViewContribution{{
					ID: "v1", Location: "sidebar.bottom", Title: "T",
				}},
				StatusBar: []domainplugin.StatusBarContribution{{
					ID: "s1", Text: "OK",
				}},
			},
		},
		RootDir: t.TempDir(),
	})

	merged := usecase.MergeContributions(registry)
	if len(merged.Views) != 1 || len(merged.StatusBar) != 1 {
		t.Fatalf("unexpected merge: %+v", merged)
	}
}
