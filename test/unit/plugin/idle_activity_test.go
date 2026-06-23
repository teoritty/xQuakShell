package plugin_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
	"ssh-client/internal/usecase"
)

func TestHostServerRecordsPluginActivity(t *testing.T) {
	var activityCount atomic.Int32
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.activity",
		Gate:     capability.NewGate(domainplugin.Manifest{}),
		OnActivity: func(pluginID string) {
			if pluginID == "com.test.activity" {
				activityCount.Add(1)
			}
		},
	})

	if _, rpcErr := server.HandleRequest(context.Background(), "ping", nil); rpcErr != nil {
		t.Fatalf("ping failed: %#v", rpcErr)
	}
	if activityCount.Load() != 1 {
		t.Fatalf("expected activity callback once, got %d", activityCount.Load())
	}
}

func TestIdleSuspendRespectsPluginRPCActivity(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := &recordingPluginHost{state: domainplugin.ProcessRunning}
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    registry,
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	manager.SetIdleTimeout(50 * time.Millisecond)

	manifest := domainplugin.Manifest{
		ID:      "com.test.idle",
		Name:    "Idle",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "idle.exe"},
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	manager.TouchActivity("com.test.idle")
	time.Sleep(80 * time.Millisecond)
	manager.TouchActivity("com.test.idle")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	manager.SuspendIdlePlugins(ctx, 60*time.Millisecond)

	if host.stopCount != 0 {
		t.Fatalf("expected plugin to stay running after recent activity, stops=%d", host.stopCount)
	}

	time.Sleep(80 * time.Millisecond)
	manager.SuspendIdlePlugins(ctx, 60*time.Millisecond)
	if host.stopCount != 1 {
		t.Fatalf("expected idle suspend after inactivity, stops=%d", host.stopCount)
	}
}

func TestIdleSuspendSkipsPluginWithActiveViewPanel(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := &recordingPluginHost{state: domainplugin.ProcessRunning}
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    registry,
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	manager.SetIdleTimeout(50 * time.Millisecond)

	manifest := domainplugin.Manifest{
		ID:      "com.test.view.idle",
		Name:    "View",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "view.exe"},
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	manager.RegisterViewPanel("com.test.view.idle", "panel.main")
	time.Sleep(80 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	manager.SuspendIdlePlugins(ctx, 60*time.Millisecond)

	if host.stopCount != 0 {
		t.Fatalf("expected running plugin with active view panel, stops=%d", host.stopCount)
	}

	manager.UnregisterViewPanel("com.test.view.idle", "panel.main")
	time.Sleep(80 * time.Millisecond)
	manager.SuspendIdlePlugins(ctx, 60*time.Millisecond)
	if host.stopCount != 1 {
		t.Fatalf("expected idle suspend after view panel closed, stops=%d", host.stopCount)
	}
}

type recordingPluginHost struct {
	state     domainplugin.ProcessState
	stopCount int
}

func (h *recordingPluginHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	h.state = domainplugin.ProcessRunning
	return nil
}

func (h *recordingPluginHost) Stop(context.Context, string, string) error {
	h.stopCount++
	h.state = domainplugin.ProcessStopped
	return nil
}

func (h *recordingPluginHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (h *recordingPluginHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}

func (h *recordingPluginHost) State(string, string) domainplugin.ProcessState {
	return h.state
}

func (h *recordingPluginHost) StopAll(context.Context) {}

func (h *recordingPluginHost) RunningInstances() []domainplugin.ProcessInstance {
	if h.state != domainplugin.ProcessRunning {
		return nil
	}
	return []domainplugin.ProcessInstance{{
		PluginID: "com.test.idle",
		State:    h.state,
	}}
}

func (h *recordingPluginHost) BindSession(string, string) error { return nil }
func (h *recordingPluginHost) UnbindSession(string, string)     {}
