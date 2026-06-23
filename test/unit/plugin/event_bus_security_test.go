package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

type sessionOwnerStub struct {
	owns map[string]bool
}

func (s sessionOwnerStub) PluginOwnsSession(pluginID, sessionID string) bool {
	return s.owns[pluginID+":"+sessionID]
}

func TestEventBusDeliversSessionEventsOnlyToOwner(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.a", Name: "A", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{Subscribe: []string{"core.session.*"}},
			},
		},
	})
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.b", Name: "B", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "b.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{Subscribe: []string{"core.session.*"}},
			},
		},
	})

	var delivered []string
	bus := usecase.NewPluginEventBus(registry, func(_ context.Context, pluginID, _ string, _ string, _ json.RawMessage) error {
		delivered = append(delivered, pluginID)
		return nil
	})
	bus.SetSessionActiveChecker(func(string) bool { return true })
	bus.SetSessionOwnershipChecker(sessionOwnerStub{owns: map[string]bool{
		"com.plugin.a:sess-1": true,
	}})

	_ = bus.Subscribe(context.Background(), "com.plugin.a", "core.session.*")
	_ = bus.Subscribe(context.Background(), "com.plugin.b", "core.session.*")

	bus.PublishCore(context.Background(), "core.session.opened", map[string]string{
		"sessionId":    "sess-1",
		"connectionId": "conn-1",
	})

	if len(delivered) != 1 || delivered[0] != "com.plugin.a" {
		t.Fatalf("expected delivery only to owner, got %v", delivered)
	}
}

func TestEventBusSkipsSessionEventsWithoutSessionID(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.a", Name: "A", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{Subscribe: []string{"core.session.*"}},
			},
		},
	})

	var delivered int
	bus := usecase.NewPluginEventBus(registry, func(_ context.Context, _ string, _ string, _ string, _ json.RawMessage) error {
		delivered++
		return nil
	})
	bus.SetSessionActiveChecker(func(string) bool { return true })
	bus.SetSessionOwnershipChecker(sessionOwnerStub{owns: map[string]bool{
		"com.plugin.a:sess-1": true,
	}})

	_ = bus.Subscribe(context.Background(), "com.plugin.a", "core.session.*")

	bus.PublishCore(context.Background(), "core.session.stateChanged", map[string]string{
		"state": "ready",
	})

	if delivered != 0 {
		t.Fatalf("expected no delivery without sessionId, got %d", delivered)
	}
}

func TestEventBusSubscribeRejectsCrossPluginChannel(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.spy", Name: "Spy", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "spy.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{
					Subscribe: []string{"plugin.com.plugin.victim.*"},
				},
			},
		},
	})

	bus := usecase.NewPluginEventBus(registry, nil)
	err := bus.Subscribe(context.Background(), "com.plugin.spy", "plugin.com.plugin.victim.events")
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected ErrCapabilityDenied, got %v", err)
	}
}

func TestEventBusSubscribeRejectsCrossPluginPattern(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.a", Name: "A", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{
					Subscribe: []string{
						"plugin.com.plugin.a.*",
						"plugin.com.plugin.b.*", // simulates manifest that bypassed validation
					},
				},
			},
		},
	})

	bus := usecase.NewPluginEventBus(registry, nil)
	if err := bus.Subscribe(context.Background(), "com.plugin.a", "plugin.com.plugin.b.*"); err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected ErrCapabilityDenied for foreign pattern, got %v", err)
	}
	if err := bus.Subscribe(context.Background(), "com.plugin.a", "plugin.com.plugin.a.*"); err != nil {
		t.Fatalf("expected own namespace subscribe ok, got %v", err)
	}
}

func TestEventBusPublishRejectsCoreChannel(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.pub", Name: "Pub", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Events: &domainplugin.EventCaps{
					Publish: []string{"plugin.com.plugin.pub.*"},
				},
			},
		},
	})

	bus := usecase.NewPluginEventBus(registry, nil)
	err := bus.PublishFromPlugin(context.Background(), "com.plugin.pub", "core.session.opened", json.RawMessage(`{}`))
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected ErrCapabilityDenied, got %v", err)
	}
}
