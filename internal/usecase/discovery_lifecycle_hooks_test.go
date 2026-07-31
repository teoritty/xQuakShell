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

// lifecycleHooks records which of the three host-internal lifecycle hooks fired. They are collected
// together because the interesting assertion is always which ONE of them ran: the three demand
// different treatments of discovery state (clear, mark stale, mark stale) and firing the wrong one
// is the bug worth catching.
type lifecycleHooks struct {
	stopped   []string
	crashed   []string
	suspended []string
}

func newHookedManager(t *testing.T, host domainplugin.ProcessHost) (*usecase.PluginManager, *lifecycleHooks) {
	t.Helper()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    usecase.NewPluginRegistry(),
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	hooks := &lifecycleHooks{}
	manager.SetProcessStoppedHandler(func(id string) { hooks.stopped = append(hooks.stopped, id) })
	manager.SetProcessCrashedHandler(func(id string) { hooks.crashed = append(hooks.crashed, id) })
	manager.SetProcessSuspendedHandler(func(id string) { hooks.suspended = append(hooks.suspended, id) })
	return manager, hooks
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
	manager, hooks := newHookedManager(t, host)

	err := manager.StopPlugin(context.Background(), "p1")
	if !errors.Is(err, failing) {
		t.Fatalf("the stop failure must still be reported to the caller, got %v", err)
	}
	if len(hooks.stopped) != 1 || hooks.stopped[0] != "p1" {
		t.Fatalf("the stopped hook must fire regardless of the stop error, got %v", hooks.stopped)
	}
	if len(hooks.crashed) != 0 || len(hooks.suspended) != 0 {
		t.Fatalf("a deliberate stop is neither a crash nor a suspension, got %+v", hooks)
	}
}

// TestIdleSuspendFiresTheSuspendedHook covers the third way a plugin stops answering. An idle
// suspension leaves the tree on screen with nobody able to confirm it, exactly like a crash — and
// without a hook the subtree stayed labelled ready and an action inside it was dispatched into a
// dead process, to fail on the ack timeout instead of being refused.
func TestIdleSuspendFiresTheSuspendedHook(t *testing.T) {
	host := &stubProcessHost{instances: []domainplugin.ProcessInstance{
		{PluginID: "p1", SessionID: "s1", State: domainplugin.ProcessRunning},
	}}
	manager, hooks := newHookedManager(t, host)

	// idleAfter of 0 makes every running instance idle immediately, so the real suspension path
	// runs rather than a test-only shortcut into it.
	manager.SuspendIdlePlugins(context.Background(), 0)

	if len(hooks.suspended) != 1 || hooks.suspended[0] != "p1" {
		t.Fatalf("the suspended hook must fire, got %v", hooks.suspended)
	}
	if len(hooks.stopped) != 0 {
		t.Fatalf("a suspension is not a stop — its subtree must be marked, not cleared, got %v", hooks.stopped)
	}
}

// TestFailedIdleSuspendLeavesTheSubtreeAlone pins the deliberate asymmetry with StopPlugin, which
// DOES fire its hook on a failed stop.
//
// The difference is who asked and what the failure means. A failed user-requested stop still means
// "gone as far as anyone can tell", and the UI already says so. A failed idle suspend means the
// process is most likely still alive and still answering — firing the hook would mark a healthy
// plugin's branches stale over a housekeeping hiccup, and the next sweep retries in a minute.
func TestFailedIdleSuspendLeavesTheSubtreeAlone(t *testing.T) {
	host := &stubProcessHost{
		stopErr: errors.New("process refused to die"),
		instances: []domainplugin.ProcessInstance{
			{PluginID: "p1", SessionID: "s1", State: domainplugin.ProcessRunning},
		},
	}
	manager, hooks := newHookedManager(t, host)

	manager.SuspendIdlePlugins(context.Background(), 0)

	if len(hooks.suspended) != 0 {
		t.Fatalf("a plugin that is still running must not be announced as suspended, got %v", hooks.suspended)
	}
	if len(hooks.stopped) != 0 || len(hooks.crashed) != 0 {
		t.Fatalf("a failed suspension is neither a stop nor a crash, got %+v", hooks)
	}
}

// TestCrashFiresTheCrashHookAndNotTheStopHook pins the split the two hooks exist for: a crash is
// transient and must only be marked, a stop is final and may be torn down. Firing the wrong one
// would either delete a subtree the supervisor is about to refill, or leave a disabled plugin's
// subtree standing.
func TestCrashFiresTheCrashHookAndNotTheStopHook(t *testing.T) {
	manager, hooks := newHookedManager(t, &stubProcessHost{})

	manager.OnProcessCrashed("p1", "s1")

	if len(hooks.crashed) != 1 || hooks.crashed[0] != "p1" {
		t.Fatalf("the crash hook must fire, got %v", hooks.crashed)
	}
	if len(hooks.stopped) != 0 {
		t.Fatalf("a crash must not fire the stopped hook, got %v", hooks.stopped)
	}
}
