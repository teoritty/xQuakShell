package plugin_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/usecase"
)

// The tests in this file cover the one property of the plugin host that nothing else asserted, and
// whose absence let a defect live in the spawner for the whole life of the project: a plugin process
// must OUTLIVE the call that started it.
//
// exec.CommandContext makes the context passed to it own the child's lifetime — cancelling it kills
// the process. The spawner used to hand it the CALLER's context, and every caller of Start passes a
// short-lived request context with a `defer cancel()`: the Wails handler behind the "Start" button,
// the ssh connector, the GitHub installer, and the supervisor, which cancelled on its own SUCCESS
// path, one line before logging that it had restarted the process. Everything downstream — the
// handshake, the registry entry, the "running" state — reported success, and the process was gone.
//
// Every existing test either kept its context alive for the whole test body or asserted only on
// state the host had already recorded, so none of them could see it.

const echoPluginID = "com.xquakshell.example-echo"

// TestAPluginProcessOutlivesTheCallThatStartedIt is the direct statement of the property. The
// context given to Start is cancelled the moment Start returns — precisely what `defer cancel()`
// does at every call site — and the process must still answer RPC afterwards.
func TestAPluginProcessOutlivesTheCallThatStartedIt(t *testing.T) {
	host, _, plugin := newEchoRig(t)

	// A separate function, so the deferred cancel fires here and not at the end of the test: the
	// point is the lifetime of the CALL, not of the test.
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := host.Start(ctx, plugin, ""); err != nil {
			t.Fatalf("start echo plugin: %v", err)
		}
	}()

	// Killing through a cancelled context is asynchronous — exec's watchdog goroutine does it — so an
	// immediate ping could race past a process that is about to die. Waiting first makes the
	// assertion below mean "still alive", not "not dead yet".
	time.Sleep(500 * time.Millisecond)

	if state := host.State(echoPluginID, ""); state != domainplugin.ProcessRunning {
		t.Fatalf("the host reports %q after the starting call returned, want %q",
			state, domainplugin.ProcessRunning)
	}

	// The state above is only the host's bookkeeping; the process itself is what is under test, so
	// the proof has to cross the process boundary.
	assertEchoAnswersPing(t, func(ctx context.Context) (json.RawMessage, error) {
		return host.Call(ctx, echoPluginID, "", "ping", nil)
	})

	// And it must keep answering: a plugin is expected to serve many calls after its start returned,
	// not to survive exactly one.
	assertEchoAnswersPing(t, func(ctx context.Context) (json.RawMessage, error) {
		return host.Call(ctx, echoPluginID, "", "ping", nil)
	})
}

// TestASupervisorRestartLeavesTheProcessRunning pins the same property at the place ADR-014 depends
// on it: crash recovery. The supervisor builds a 15s context, restarts the plugin, activates it and
// then cancels — the process it just brought back used to die on that cancel, so the promised
// "process returns, observed set is replayed, branches refill" never happened for any plugin.
//
// This is not a duplicate of TestASuccessfulRestartOfASessionlessPluginIsNotAFailedAttempt: that one
// counts attempts against a stub host, this one runs a real child process and asks whether it is
// still there.
func TestASupervisorRestartLeavesTheProcessRunning(t *testing.T) {
	host, manager, _ := newEchoRig(t)
	// The plugin is held by something (a session, a discovery binding) — otherwise the supervisor
	// correctly declines to restart it at all.
	manager.SetPluginRetentionChecker(func(pluginID string) bool { return pluginID == echoPluginID })

	supervisor := usecase.NewPluginSupervisor(manager)
	gaveUp := make(chan string, 1)
	supervisor.SetGaveUpHandler(func(pluginID string) { gaveUp <- pluginID })

	supervisor.HandleCrash(echoPluginID, "sess-restart")

	deadline := time.Now().Add(30 * time.Second)
	for host.State(echoPluginID, "") != domainplugin.ProcessRunning && time.Now().Before(deadline) {
		select {
		case pluginID := <-gaveUp:
			t.Fatalf("the supervisor gave up on %q instead of restarting it", pluginID)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if host.State(echoPluginID, "") != domainplugin.ProcessRunning {
		t.Fatal("the supervisor never brought the crashed plugin back up")
	}

	// Past the supervisor's own cancel(), which happens between the successful activate and the
	// "restarted process" log line.
	time.Sleep(1 * time.Second)

	assertEchoAnswersPing(t, func(ctx context.Context) (json.RawMessage, error) {
		return manager.CallForSession(ctx, echoPluginID, "sess-restart", "ping", nil)
	})
}

// newEchoRig builds the real host over the real echo fixture binary. Fakes are useless here: the
// defect lives in exec.CommandContext, so only an OS process can show it.
func newEchoRig(t *testing.T) (*infraplugin.ProcessHost, *usecase.PluginManager, domainplugin.InstalledPlugin) {
	t.Helper()
	pluginDir := buildExampleEchoPlugin(t)

	registry := usecase.NewPluginRegistry()
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionAuthorizer: usecase.NewPluginSessionAuthorizer(registry),
	})
	manager := newTestPluginManager(t, registry, host)
	if err := manager.DiscoverPlugins(infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)}).Discover); err != nil {
		t.Fatalf("discover echo fixture: %v", err)
	}
	t.Cleanup(func() { host.StopAll(context.Background()) })

	plugin, err := registry.Get(echoPluginID)
	if err != nil {
		t.Fatalf("echo fixture did not reach the registry: %v", err)
	}
	return host, manager, plugin
}

func assertEchoAnswersPing(t *testing.T, call func(ctx context.Context) (json.RawMessage, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	raw, err := call(ctx)
	if err != nil {
		t.Fatalf("the plugin process did not answer ping after the call that started it returned: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode ping result %s: %v", raw, err)
	}
	if result["pong"] != "ok" {
		t.Fatalf("unexpected ping result: %v", result)
	}
}
