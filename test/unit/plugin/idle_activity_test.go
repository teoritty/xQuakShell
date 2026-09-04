package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/ipc"
	"xquakshell/internal/usecase"
)

func TestHostServerRecordsPluginActivity(t *testing.T) {
	var activityCount atomic.Int32
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.activity",
		Gate:     newGate(t, domainplugin.Manifest{}),
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

// activityProbe builds a host server for a manifest that declares the channel capability, so
// channel.open passes the gate and is decided by the session handler instead — which is the only
// arrangement in which the bug this file guards could ever have happened.
func activityProbe(t *testing.T, handlerErr error) (*ipc.HostServer, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	manifest := domainplugin.Manifest{
		ID:      "com.test.activity",
		Name:    "Activity",
		Version: "1.0.0",
		Capabilities: domainplugin.CapabilitySet{
			Channel: &domainplugin.ChannelCaps{Purposes: []string{domainplugin.PurposeExec}},
		},
	}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.activity",
		Gate:     newGate(t, manifest),
		Sessions: sessionRPCFunc(func(context.Context, string, string, json.RawMessage) (json.RawMessage, error) {
			return nil, handlerErr
		}),
		OnActivity: func(string) { calls.Add(1) },
	})
	return server, &calls
}

// TestDeniedSessionRPCIsNotActivity is the regression guard for a plugin that kept itself alive by
// being refused.
//
// channel.open clears the capability gate — the manifest does declare the channel capability — and
// is refused deeper, by the session authorizer, because the plugin holds no binding for the session
// it named. Activity used to be recorded between those two points, so every refusal reset the idle
// clock and the sweep could never reclaim the process.
func TestDeniedSessionRPCIsNotActivity(t *testing.T) {
	server, calls := activityProbe(t, domainplugin.ErrSessionNotBound)

	_, rpcErr := server.HandleRequest(context.Background(), "channel.open", mustJSON(map[string]string{
		"purpose":         domainplugin.PurposeExec,
		"parentSessionId": "not-ours",
	}))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("rpcErr = %#v, want capability denied -32001", rpcErr)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("activity recorded %d times for a refused call, want 0; a refusal must not reset the idle clock", got)
	}
}

// TestGateDenialIsNotActivity covers the shallower refusal: the manifest never granted the
// capability, so the call never reaches a handler at all.
func TestGateDenialIsNotActivity(t *testing.T) {
	var calls atomic.Int32
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID:   "com.test.activity",
		Gate:       newGate(t, domainplugin.Manifest{ID: "com.test.activity"}),
		OnActivity: func(string) { calls.Add(1) },
	})

	_, rpcErr := server.HandleRequest(context.Background(), "fs.read", mustJSON(map[string]string{"path": "x"}))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("rpcErr = %#v, want capability denied -32001", rpcErr)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("activity recorded %d times for a gate denial, want 0", got)
	}
}

// TestAttemptedButFailedRPCIsStillActivity pins the other half of the rule, and it is the half a
// careless fix loses: narrowing the record to successful calls only would suspend a healthy plugin
// whose work happens to be failing.
func TestAttemptedButFailedRPCIsStillActivity(t *testing.T) {
	server, calls := activityProbe(t, errors.New("the daemon refused the exec"))

	_, rpcErr := server.HandleRequest(context.Background(), "channel.open", mustJSON(map[string]string{
		"purpose":         domainplugin.PurposeExec,
		"parentSessionId": "ours",
	}))
	if rpcErr == nil || rpcErr.Code != -32603 {
		t.Fatalf("rpcErr = %#v, want internal failure -32603", rpcErr)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("activity recorded %d times for an attempted call, want 1", got)
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

func (h *recordingPluginHost) Start(context.Context, domainplugin.InstalledPlugin, string, domainplugin.SandboxPolicy) error {
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

func (h *recordingPluginHost) CallWithTimeout(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage, _ time.Duration) (json.RawMessage, error) {
	return h.Call(ctx, pluginID, sessionID, method, params)
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
