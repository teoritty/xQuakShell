package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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

	mu     sync.Mutex
	starts int
}

func (h *stubProcessHost) Start(context.Context, domainplugin.InstalledPlugin, string) error {
	h.mu.Lock()
	h.starts++
	h.mu.Unlock()
	return nil
}

func (h *stubProcessHost) startCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.starts
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
// BindSession mirrors the production authorizer on the one rule this file depends on: an empty
// session id authorizes nothing and is refused. A stub that accepted it would let the supervisor's
// broken branch look healthy here while failing in the app — which is how the defect this stub now
// reproduces survived a whole round of tests.
func (h *stubProcessHost) BindSession(_, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return domainplugin.ErrSessionNotBound
	}
	return nil
}
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

// A discovery plugin holds no session and owns no view panel, so every "is anyone using this?"
// check in the host answered no. The two consequences below were both live in production: the idle
// sweeper reclaimed the plugin after five quiet minutes — quiet being the normal state of a tree
// the user has finished expanding — and the supervisor refused to restart it after a crash, so the
// give-up path that turns stale branches into a stated failure was never reached either.

// TestIdleSweepSparesAPluginHeldOnlyByADiscoveryBinding: idleness is not grounds for reclaiming a
// plugin that is still drawing something on screen.
func TestIdleSweepSparesAPluginHeldOnlyByADiscoveryBinding(t *testing.T) {
	host := &stubProcessHost{instances: []domainplugin.ProcessInstance{
		{PluginID: "p1", SessionID: "s1", State: domainplugin.ProcessRunning},
	}}
	manager, hooks := newHookedManager(t, host)
	manager.SetPluginRetentionChecker(func(pluginID string) bool { return pluginID == "p1" })

	// idleAfter of 0 makes every running instance idle immediately, so only the retention check can
	// save it.
	manager.SuspendIdlePlugins(context.Background(), 0)

	if len(hooks.suspended) != 0 {
		t.Fatalf("a plugin drawing a subtree must not be suspended for being quiet, got %v", hooks.suspended)
	}
}

// TestIdleSweepStillSuspendsAPluginNobodyHolds is the other half: without it the test above would
// pass against a sweeper that had simply stopped suspending anything.
func TestIdleSweepStillSuspendsAPluginNobodyHolds(t *testing.T) {
	host := &stubProcessHost{instances: []domainplugin.ProcessInstance{
		{PluginID: "p1", SessionID: "s1", State: domainplugin.ProcessRunning},
	}}
	manager, hooks := newHookedManager(t, host)
	manager.SetPluginRetentionChecker(func(string) bool { return false })

	manager.SuspendIdlePlugins(context.Background(), 0)

	if len(hooks.suspended) != 1 {
		t.Fatalf("an idle plugin nobody holds must still be reclaimed, got %v", hooks.suspended)
	}
}

// recordingRecoverer stands in for the session bridge. Production always sets one, and the tests
// that omitted it were the reason the bug below survived: with a nil recoverer the whole
// session-recovery block is skipped, which is exactly the branch that was broken.
type recordingRecoverer struct {
	mu        sync.Mutex
	recovered []string
	failed    []string
}

func (r *recordingRecoverer) RecoverPluginSession(_ context.Context, _, sessionID string) error {
	r.mu.Lock()
	r.recovered = append(r.recovered, sessionID)
	r.mu.Unlock()
	return nil
}

func (r *recordingRecoverer) FailPluginSessionRecovery(_, sessionID string) {
	r.mu.Lock()
	r.failed = append(r.failed, sessionID)
	r.mu.Unlock()
}

func (r *recordingRecoverer) calls() (recovered, failed []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.recovered...), append([]string(nil), r.failed...)
}

// TestASuccessfulRestartOfASessionlessPluginIsNotAFailedAttempt is the branch nothing covered: a
// restart that WORKS.
//
// A shared-scope process is reported as crashed with an empty sessionID, because the host has no
// session to name for it. The supervisor then tried to re-authorize that empty session, the
// authorizer refused it with ErrSessionNotBound — correctly, an empty session id authorizes
// nothing — and the loop counted the successful restart as a failed attempt. Three good restarts
// later it announced that it had given up, and the give-up handler painted the plugin's branches
// error, meaning "nobody is coming", over a plugin that was running and had already refilled them.
//
// Every earlier test reached give-up through a plugin that was NOT in the registry, so the restart
// failed for real and this path never ran.
func TestASuccessfulRestartOfASessionlessPluginIsNotAFailedAttempt(t *testing.T) {
	host := &stubProcessHost{}
	manager, _ := newHookedManager(t, host)
	if err := manager.Registry().Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "Sessionless", Version: "1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	// Held by a discovery binding — no session of its own, which is the whole point.
	manager.SetPluginRetentionChecker(func(pluginID string) bool { return pluginID == "p1" })

	supervisor := usecase.NewPluginSupervisor(manager)
	recoverer := &recordingRecoverer{}
	supervisor.SetRecoverer(recoverer)
	gaveUp := make(chan string, 1)
	supervisor.SetGaveUpHandler(func(pluginID string) { gaveUp <- pluginID })

	supervisor.HandleCrash("p1", "")

	deadline := time.Now().Add(10 * time.Second)
	for host.startCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if host.startCount() == 0 {
		t.Fatal("the crashed plugin was never restarted")
	}

	// Three failed attempts take ~1.4 s of backoff; this waits past that.
	select {
	case pluginID := <-gaveUp:
		t.Fatalf("%q restarted successfully and was still reported as abandoned; its branches would "+
			"be painted error over a running plugin", pluginID)
	case <-time.After(3 * time.Second):
	}

	if starts := host.startCount(); starts != 1 {
		t.Fatalf("a successful restart must be attempted once, not retried: %d starts", starts)
	}
	recovered, failed := recoverer.calls()
	if len(recovered) != 0 {
		t.Fatalf("there is no session to reconnect for a shared-scope process, got %d call(s) naming %q",
			len(recovered), recovered)
	}
	if len(failed) != 0 {
		t.Fatalf("no session recovery was attempted, so none can have failed, got %d call(s) naming %q",
			len(failed), failed)
	}
}

// TestSupervisorRestartsAPluginHeldOnlyByADiscoveryBinding: HandleCrash used to return on the first
// line for a plugin with no sessions, so the restart loop never ran, the attempts never ran out,
// and MarkPluginUnrecoverable had no production trigger at all. Reaching the give-up handler here
// is the proof that the loop ran.
func TestSupervisorRestartsAPluginHeldOnlyByADiscoveryBinding(t *testing.T) {
	manager, _ := newHookedManager(t, &stubProcessHost{})
	manager.SetPluginRetentionChecker(func(pluginID string) bool { return pluginID == "p1" })
	supervisor := usecase.NewPluginSupervisor(manager)

	gaveUp := make(chan string, 1)
	supervisor.SetGaveUpHandler(func(pluginID string) { gaveUp <- pluginID })

	supervisor.HandleCrash("p1", "s1")

	select {
	case <-gaveUp:
	case <-time.After(30 * time.Second):
		t.Fatal("a crashed plugin held by a discovery binding was never restarted or reported")
	}
}

// TestSupervisorIgnoresAPluginNobodyHolds keeps the early return meaningful: a plugin with no
// sessions and no retention holder is genuinely finished, and restarting it would resurrect
// something nobody asked for.
func TestSupervisorIgnoresAPluginNobodyHolds(t *testing.T) {
	manager, _ := newHookedManager(t, &stubProcessHost{})
	supervisor := usecase.NewPluginSupervisor(manager)

	gaveUp := make(chan string, 1)
	supervisor.SetGaveUpHandler(func(pluginID string) { gaveUp <- pluginID })

	supervisor.HandleCrash("p1", "s1")

	select {
	case pluginID := <-gaveUp:
		t.Fatalf("nothing holds %q; it must not be restarted at all", pluginID)
	case <-time.After(2 * time.Second):
	}
}

// TestSupervisorReportsThePluginItGaveUpOn wires the last lifecycle transition discovery cares
// about. The crash hook fires on every crash, including the ones the supervisor immediately
// repairs, so it cannot answer "is this subtree coming back?" — only the supervisor knows when the
// answer becomes no, and before this hook existed nobody was told.
//
// The restart fails because the plugin is not in the registry: EnsureRunningForSession cannot
// resolve it, every attempt fails, and the loop runs out — the real path, not a shortcut into it.
func TestSupervisorReportsThePluginItGaveUpOn(t *testing.T) {
	manager, _ := newHookedManager(t, &stubProcessHost{})
	// HandleCrash only acts while sessions are still open: a plugin nobody is using is left dead.
	manager.SessionOpened("p1")
	supervisor := usecase.NewPluginSupervisor(manager)

	gaveUp := make(chan string, 1)
	supervisor.SetGaveUpHandler(func(pluginID string) { gaveUp <- pluginID })

	supervisor.HandleCrash("p1", "s1")

	select {
	case pluginID := <-gaveUp:
		if pluginID != "p1" {
			t.Fatalf("the handler must name the abandoned plugin, got %q", pluginID)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the supervisor exhausted its attempts without telling anyone")
	}
}

// TestSupervisorWithoutAGiveUpHandlerStillGivesUp: the handler is optional wiring, and a nil one
// must not turn an exhausted restart loop into a panic on a background goroutine.
func TestSupervisorWithoutAGiveUpHandlerStillGivesUp(t *testing.T) {
	manager, _ := newHookedManager(t, &stubProcessHost{})
	manager.SessionOpened("p1")
	supervisor := usecase.NewPluginSupervisor(manager)

	done := make(chan struct{})
	supervisor.SetGaveUpHandler(nil)
	go func() {
		supervisor.HandleCrash("p1", "s1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("HandleCrash must return promptly; the restart loop runs in the background")
	}
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
