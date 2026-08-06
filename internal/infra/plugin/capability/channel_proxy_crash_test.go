package capability

import (
	"context"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestChannelProxy_CloseAllInvokesBackendCloseRemoteForEveryChannel proves CloseAll (the
// process-exit/crash teardown path, ADR-011) signals every owned channel's backend to
// tear down its remote end, not just drop the local ownership map — otherwise a remote docker
// exec / relay conn would leak past the plugin process's own lifetime.
func TestChannelProxy_CloseAllInvokesBackendCloseRemoteForEveryChannel(t *testing.T) {
	backendA := &fakeChannelBackend{}
	backendB := &fakeChannelBackend{}
	calls := 0
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) {
		calls++
		if calls == 1 {
			return backendA, nil
		}
		return backendB, nil
	}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())
	ctx := context.Background()

	if _, err := proxy.Open(ctx, openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open A: %v", err)
	}
	if _, err := proxy.Open(ctx, openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open B: %v", err)
	}

	proxy.CloseAll()

	if backendA.closeCalls.Load() != 1 {
		t.Fatalf("backend A CloseRemote calls = %d, want 1", backendA.closeCalls.Load())
	}
	if backendB.closeCalls.Load() != 1 {
		t.Fatalf("backend B CloseRemote calls = %d, want 1", backendB.closeCalls.Load())
	}

	proxy.mu.Lock()
	remaining := len(proxy.channels)
	proxy.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected 0 owned channels after CloseAll, got %d", remaining)
	}
}

// TestChannelProxy_CloseAllWorksWithoutEverCallingCloseSession simulates a plugin-process
// crash while its parent session stays open: only CloseAll is invoked, CloseSession is never
// called. If process-crash teardown silently depended on CloseSession, this would leave the
// backend's remote end open.
func TestChannelProxy_CloseAllWorksWithoutEverCallingCloseSession(t *testing.T) {
	backend := &fakeChannelBackend{}
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())
	ctx := context.Background()

	if _, err := proxy.Open(ctx, openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Process-crash teardown only: CloseSession must never be invoked here.
	proxy.CloseAll()

	if backend.closeCalls.Load() != 1 {
		t.Fatalf("CloseRemote calls = %d, want 1 (CloseAll alone must tear down the channel)", backend.closeCalls.Load())
	}
}

// TestChannelProxy_CloseSessionWorksWithoutEverCallingCloseAll proves the session-cascade close
// (ADR-011) works on a still-alive process, i.e. it does not rely on process-crash
// teardown (CloseAll) ever running.
func TestChannelProxy_CloseSessionWorksWithoutEverCallingCloseAll(t *testing.T) {
	backend := &fakeChannelBackend{}
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())
	ctx := context.Background()

	if _, err := proxy.Open(ctx, openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Session-cascade close only: CloseAll must never be invoked here; the process is alive.
	proxy.CloseSession("sess-1")

	if backend.closeCalls.Load() != 1 {
		t.Fatalf("CloseRemote calls = %d, want 1 (CloseSession alone must tear down the channel)", backend.closeCalls.Load())
	}

	proxy.mu.Lock()
	remaining := len(proxy.channels)
	proxy.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected 0 owned channels after CloseSession, got %d", remaining)
	}
}
