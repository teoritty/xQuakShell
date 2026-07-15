package plugin_test

import (
	"context"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/usecase"
)

type settingsReader struct {
	disabled map[string]bool
}

func mustRegister(t *testing.T, registry *usecase.PluginRegistry, plugin domainplugin.InstalledPlugin) {
	t.Helper()
	if err := registry.Register(plugin); err != nil {
		t.Fatalf("register plugin %q: %v", plugin.Manifest.ID, err)
	}
}

func (s settingsReader) PluginSettings() (domain.PluginSettings, error) {
	return domain.PluginSettings{Disabled: s.disabled}, nil
}

func TestManualStartDeniedWhenPluginDisabled(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: t.TempDir()})
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           host,
		InstallRoot:    t.TempDir(),
		SettingsReader: settingsReader{disabled: map[string]bool{"com.test.disabled": true}},
	})

	manifest := domainplugin.Manifest{
		ID:      "com.test.disabled",
		Name:    "Disabled",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: "go-binary", Entry: "p.exe"},
	}
	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: manifest})

	err := manager.StartPluginManual(context.Background(), "com.test.disabled")
	if err != domainplugin.ErrPluginDisabled {
		t.Fatalf("expected ErrPluginDisabled, got %v", err)
	}
}

func TestManualStartAuthorizeWhenPluginEnabled(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: t.TempDir()})
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           host,
		InstallRoot:    t.TempDir(),
		SettingsReader: settingsReader{},
	})

	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{
		ID:      "com.test.enabled",
		Name:    "Enabled",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: "go-binary", Entry: "p.exe"},
		ActivationEvents: []string{"onManual"},
	}})

	if err := manager.AuthorizeStart("com.test.enabled", usecase.StartReasonManual, ""); err != nil {
		t.Fatalf("expected manual start authorization, got %v", err)
	}
}

func TestManualStartDeniedWithoutOnManual(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: t.TempDir()})
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           host,
		InstallRoot:    t.TempDir(),
		SettingsReader: settingsReader{},
	})

	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{
		ID:      "com.test.nomanual",
		Name:    "No Manual",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: "go-binary", Entry: "p.exe"},
		ActivationEvents: []string{"onStartup"},
	}})

	if err := manager.AuthorizeStart("com.test.nomanual", usecase.StartReasonManual, ""); err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected capability denied, got %v", err)
	}
}

func TestViewStartDeniedWithoutOnView(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    registry,
		Host:        infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: t.TempDir()}),
		InstallRoot: t.TempDir(),
	})

	mustRegister(t, registry, domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{
		ID:      "com.test.view",
		Name:    "View",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: "go-binary", Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			Views: []domainplugin.ViewContribution{{ID: "panel", Location: "sidebar.bottom", Title: "P", Type: "webview", Entry: "ui/index.html"}},
		},
		ActivationEvents: []string{"onStartup"},
	}})

	err := manager.AuthorizeStart("com.test.view", usecase.StartReasonView, "panel")
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected capability denied without onView, got %v", err)
	}
}
