package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

func TestViewRelayRejectsMissingToken(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	relay := usecase.NewPluginViewRelay(nil, registry)

	err := relay.RelayMessage(context.Background(), "", json.RawMessage(`{}`))
	if !errors.Is(err, domainplugin.ErrViewRelayTokenInvalid) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestViewRelayRejectsUnknownToken(t *testing.T) {
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

	manager := newTestPluginManager(t, registry, nil)
	relay := usecase.NewPluginViewRelay(manager, registry)

	token, err := relay.PreparePanel("com.test.views", "panel.one")
	if err != nil {
		t.Fatal(err)
	}
	relay.ReleasePanel(token)

	err = relay.RelayMessage(context.Background(), token, json.RawMessage(`{"x":1}`))
	if !errors.Is(err, domainplugin.ErrViewRelayTokenInvalid) {
		t.Fatalf("expected invalid token after release, got %v", err)
	}
}

func TestViewRelayPrepareReturnsToken(t *testing.T) {
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

	manager := newTestPluginManager(t, registry, nil)
	relay := usecase.NewPluginViewRelay(manager, registry)

	token, err := relay.PreparePanel("com.test.views", "panel.one")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("expected long relay token, got %q", token)
	}
	relay.ReleasePanel(token)
}
