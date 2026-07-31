package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// stubProcessHost is the smallest ProcessHost that can fail a stop on demand. Everything else is a
// no-op: these tests are about which lifecycle hooks fire, not about processes.
type stubProcessHost struct {
	stopErr   error
	instances []domainplugin.ProcessInstance
}

func (h *stubProcessHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	return nil
}
func (h *stubProcessHost) Stop(context.Context, string, string) error { return h.stopErr }
func (h *stubProcessHost) StopAll(context.Context)                    {}
func (h *stubProcessHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (h *stubProcessHost) CallWithTimeout(context.Context, string, string, string, json.RawMessage, time.Duration) (json.RawMessage, error) {
	return nil, nil
}
func (h *stubProcessHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (h *stubProcessHost) State(string, string) domainplugin.ProcessState {
	return domainplugin.ProcessStopped
}
func (h *stubProcessHost) RunningInstances() []domainplugin.ProcessInstance { return h.instances }
func (h *stubProcessHost) BindSession(string, string) error                 { return nil }
func (h *stubProcessHost) UnbindSession(string, string)                     {}

func newHookedManager(t *testing.T, host domainplugin.ProcessHost) (*usecase.PluginManager, *[]string, *[]string) {
	t.Helper()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    usecase.NewPluginRegistry(),
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	var stopped, crashed []string
	manager.SetProcessStoppedHandler(func(pluginID string) { stopped = append(stopped, pluginID) })
	manager.SetProcessCrashedHandler(func(pluginID string) { crashed = append(crashed, pluginID) })
	return manager, &stopped, &crashed
}

// TestStopPluginTearsDownEvenWhenStoppingFailed is the gap between "the user disabled this plugin"
// and "every OS process actually exited". Presentation discards StopPlugin's error, so a teardown
// that only ran on the happy path would leave a disabled plugin's discovery subtree in the tree
// with nothing on screen to explain it.
func TestStopPluginTearsDownEvenWhenStoppingFailed(t *testing.T) {
	failing := errors.New("process refused to die")
	host := &stubProcessHost{
		stopErr:   failing,
		instances: []domainplugin.ProcessInstance{{PluginID: "p1", SessionID: "s1"}},
	}
	manager, stopped, crashed := newHookedManager(t, host)

	err := manager.StopPlugin(context.Background(), "p1")
	if !errors.Is(err, failing) {
		t.Fatalf("the stop failure must still be reported to the caller, got %v", err)
	}
	if len(*stopped) != 1 || (*stopped)[0] != "p1" {
		t.Fatalf("the stopped hook must fire regardless of the stop error, got %v", *stopped)
	}
	if len(*crashed) != 0 {
		t.Fatalf("a deliberate stop is not a crash, got %v", *crashed)
	}
}

// TestCrashFiresTheCrashHookAndNotTheStopHook pins the split the two hooks exist for: a crash is
// transient and must only be marked, a stop is final and may be torn down. Firing the wrong one
// would either delete a subtree the supervisor is about to refill, or leave a disabled plugin's
// subtree standing.
func TestCrashFiresTheCrashHookAndNotTheStopHook(t *testing.T) {
	manager, stopped, crashed := newHookedManager(t, &stubProcessHost{})

	manager.OnProcessCrashed("p1", "s1")

	if len(*crashed) != 1 || (*crashed)[0] != "p1" {
		t.Fatalf("the crash hook must fire, got %v", *crashed)
	}
	if len(*stopped) != 0 {
		t.Fatalf("a crash must not fire the stopped hook, got %v", *stopped)
	}
}
