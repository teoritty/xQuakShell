package plugin_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

type disabledPluginHost struct {
	state domainplugin.ProcessState
}

func (disabledPluginHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	return nil
}
func (disabledPluginHost) Stop(context.Context, string, string) error { return nil }
func (disabledPluginHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (disabledPluginHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (h disabledPluginHost) State(string, string) domainplugin.ProcessState { return h.state }
func (disabledPluginHost) StopAll(context.Context)                        {}
func (disabledPluginHost) RunningInstances() []domainplugin.ProcessInstance {
	return nil
}
func (disabledPluginHost) BindSession(string, string) error { return nil }
func (disabledPluginHost) UnbindSession(string, string)     {}

func TestEnsureRunningForSessionRejectsDisabledPlugin(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.disabled", Name: "D", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		},
	})

	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           disabledPluginHost{state: domainplugin.ProcessDiscovered},
		InstallRoot:    t.TempDir(),
		SettingsReader: settingsReader{disabled: map[string]bool{"com.test.disabled": true}},
	})

	err := manager.EnsureRunningForSession(context.Background(), "com.test.disabled", "sess-1")
	if err != domainplugin.ErrPluginDisabled {
		t.Fatalf("expected ErrPluginDisabled, got %v", err)
	}
}

func TestSupervisorSkipsRestartWhenPluginDisabled(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.disabled", Name: "D", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		},
	})

	host := disabledPluginHost{state: domainplugin.ProcessDiscovered}
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           host,
		InstallRoot:    t.TempDir(),
		SettingsReader: settingsReader{disabled: map[string]bool{"com.test.disabled": true}},
	})
	manager.SessionOpened("com.test.disabled")

	supervisor := usecase.NewPluginSupervisor(manager)
	supervisor.HandleCrash("com.test.disabled", "sess-1")

	time.Sleep(300 * time.Millisecond)

	if host.State("com.test.disabled", "sess-1") != domainplugin.ProcessDiscovered {
		t.Fatalf("expected supervisor not to restart disabled plugin")
	}
}
