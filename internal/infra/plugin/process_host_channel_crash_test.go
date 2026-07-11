package plugin

import (
	"context"
	"sync/atomic"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

// fakeCrashChannelBackend records CloseRemote calls without touching any session state, so
// tests can prove a plugin-process exit tears down the remote end of a channel with no
// dependency on session-close machinery.
type fakeCrashChannelBackend struct {
	closeCalls atomic.Int32
}

func (f *fakeCrashChannelBackend) Authorize(purpose, parentSessionID, hint string) error { return nil }

func (f *fakeCrashChannelBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	return nil
}

func (f *fakeCrashChannelBackend) CloseRemote() error {
	f.closeCalls.Add(1)
	return nil
}

func closedReaper(exitErr error) *processReaper {
	r := &processReaper{exited: make(chan struct{})}
	r.err = exitErr
	close(r.exited)
	return r
}

// TestWaitProcess_ClosesChannelsOnCrash_WithoutSessionClose is the Stage 4b invariant: a plugin
// process crashing while its parent SSH session stays open must close every channel that process
// owned, and tear down the backend's remote end, without any session-close event ever occurring.
func TestWaitProcess_ClosesChannelsOnCrash_WithoutSessionClose(t *testing.T) {
	backend := &fakeCrashChannelBackend{}
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := capability.NewChannelProxy("p1", caps, resolve, nil)

	if _, err := proxy.Open(context.Background(), openParamsJSON("sess-1", "exec")); err != nil {
		t.Fatalf("open: %v", err)
	}

	bus := capability.NewChannelBus()
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir(), ChannelBus: bus})

	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "P1", Version: "1"},
	}
	key := processKey(plugin, "sess-parent")
	mp := &managedProcess{
		key:       key,
		plugin:    plugin,
		sessionID: "sess-parent",
		state:     domainplugin.ProcessRunning,
		channels:  proxy,
		reaper:    closedReaper(errCrash),
	}
	bus.Register(key, proxy)

	host.mu.Lock()
	host.processes[key] = mp
	host.mu.Unlock()

	// No session-close event of any kind occurs in this test — waitProcess alone must tear
	// down the channel. The parent session (sess-parent) is never touched.
	host.waitProcess(key, mp)

	if backend.closeCalls.Load() != 1 {
		t.Fatalf("backend CloseRemote calls = %d, want 1 (process crash must close its own channels)", backend.closeCalls.Load())
	}

	host.mu.Lock()
	_, stillTracked := host.processes[key]
	host.mu.Unlock()
	if stillTracked {
		t.Fatal("crashed process must be removed from the process table")
	}
}

// TestWaitProcess_ClosesChannelsOnCleanExit proves the teardown is not crash-specific: any
// process exit (clean stop or crash) closes that process's channels, matching the
// TunnelDialProxy/NetProxy precedent already wired into managedProcess.closeResources.
func TestWaitProcess_ClosesChannelsOnCleanExit(t *testing.T) {
	backend := &fakeCrashChannelBackend{}
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := capability.NewChannelProxy("p1", caps, resolve, nil)

	if _, err := proxy.Open(context.Background(), openParamsJSON("sess-1", "exec")); err != nil {
		t.Fatalf("open: %v", err)
	}

	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})
	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "P1", Version: "1"},
	}
	key := processKey(plugin, "sess-parent")
	mp := &managedProcess{
		key:       key,
		plugin:    plugin,
		sessionID: "sess-parent",
		state:     domainplugin.ProcessRunning,
		channels:  proxy,
		reaper:    closedReaper(nil), // nil ExitErr => clean stop, not a crash
	}

	host.mu.Lock()
	host.processes[key] = mp
	host.mu.Unlock()

	host.waitProcess(key, mp)

	if backend.closeCalls.Load() != 1 {
		t.Fatalf("backend CloseRemote calls = %d, want 1 (clean exit must also close channels)", backend.closeCalls.Load())
	}
}

var errCrash = errProcessCrashedForTest{}

type errProcessCrashedForTest struct{}

func (errProcessCrashedForTest) Error() string { return "simulated plugin crash" }

func openParamsJSON(parentSessionID, purpose string) []byte {
	return []byte(`{"parentSessionId":"` + parentSessionID + `","purpose":"` + purpose + `"}`)
}
