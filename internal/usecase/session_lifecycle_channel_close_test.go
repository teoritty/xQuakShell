package usecase

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// seqRecorder hands out monotonically increasing sequence numbers so tests can assert ordering
// between events that happen on different goroutines/components without relying on wall-clock
// timestamps (which can tie on fast machines).
type seqRecorder struct {
	mu   sync.Mutex
	next int
}

func (r *seqRecorder) next_() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	return r.next
}

// fakeCloseSSHClient records the sequence number at which Close() was called.
type fakeCloseSSHClient struct {
	seq        *seqRecorder
	mu         sync.Mutex
	closeAt    int
	closeCalls int
}

func (f *fakeCloseSSHClient) OpenDirectTCP(context.Context, string) (net.Conn, error) { return nil, nil }
func (f *fakeCloseSSHClient) ListenTCP(context.Context, string) (net.Listener, error) { return nil, nil }
func (f *fakeCloseSSHClient) NewSession() (*gossh.Session, error)                     { return nil, nil }
func (f *fakeCloseSSHClient) Client() *gossh.Client                                   { return nil }
func (f *fakeCloseSSHClient) KeepAlive() error                                        { return nil }
func (f *fakeCloseSSHClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	f.closeAt = f.seq.next_()
	return nil
}

var _ domain.SSHClient = (*fakeCloseSSHClient)(nil)

// fakeChannelBus is a test double for domainplugin.ChannelSessionCloser. It records, per
// sessionID, the sequence number at which CloseSession was invoked, and lets tests register a
// per-channel-backend close hook so ordering vs. sshClient.Close() can be asserted.
type fakeChannelBus struct {
	seq *seqRecorder

	mu           sync.Mutex
	closedAt     map[string]int
	closeCalls   map[string]int
	onCloseHooks map[string]func()
}

func newFakeChannelBus(seq *seqRecorder) *fakeChannelBus {
	return &fakeChannelBus{
		seq:          seq,
		closedAt:     make(map[string]int),
		closeCalls:   make(map[string]int),
		onCloseHooks: make(map[string]func()),
	}
}

func (b *fakeChannelBus) CloseSession(sessionID string) {
	b.mu.Lock()
	b.closeCalls[sessionID]++
	b.closedAt[sessionID] = b.seq.next_()
	hook := b.onCloseHooks[sessionID]
	b.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (b *fakeChannelBus) setHook(sessionID string, hook func()) {
	b.mu.Lock()
	b.onCloseHooks[sessionID] = hook
	b.mu.Unlock()
}

func (b *fakeChannelBus) callsFor(sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeCalls[sessionID]
}

func (b *fakeChannelBus) closedAtFor(sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closedAt[sessionID]
}

func channelCloseTestLifecycle(t *testing.T) (*SessionLifecycleService, *SessionRegistry) {
	t.Helper()
	registry := NewSessionRegistry()
	lifecycle := NewSessionLifecycleService(SessionLifecycleConfig{
		Registry: registry,
	})
	return lifecycle, registry
}

func putTestSessionEntry(registry *SessionRegistry, sessionID string, sshClient domain.SSHClient) {
	ctx, cancel := context.WithCancel(context.Background())
	entry := newSessionEntry(domain.ConnectionSession{
		SessionID: sessionID,
		State:     domain.SessionReady,
	}, ctx, cancel, "conn-1")
	entry.sshClient = sshClient
	registry.Put(sessionID, entry)
}

// TestCloseSession_ChannelCascade_BeforeSSHClientClose proves the ADR-011 §Session lifecycle
// coupling invariant: closing a parent session synchronously closes every channel bound to it
// BEFORE the session's ssh client is torn down (exec channels ride that client).
func TestCloseSession_ChannelCascade_BeforeSSHClientClose(t *testing.T) {
	seq := &seqRecorder{}
	lifecycle, registry := channelCloseTestLifecycle(t)
	bus := newFakeChannelBus(seq)
	lifecycle.SetChannelBus(bus)

	sshClient := &fakeCloseSSHClient{seq: seq}
	const sessionID = "sess-cascade-1"
	putTestSessionEntry(registry, sessionID, sshClient)

	if err := lifecycle.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	if bus.callsFor(sessionID) != 1 {
		t.Fatalf("expected channel bus CloseSession called exactly once, got %d", bus.callsFor(sessionID))
	}
	if sshClient.closeCalls != 1 {
		t.Fatalf("expected sshClient.Close called exactly once, got %d", sshClient.closeCalls)
	}

	channelSeq := bus.closedAtFor(sessionID)
	sshSeq := sshClient.closeAt
	if channelSeq == 0 || sshSeq == 0 {
		t.Fatalf("expected both close hooks to run, got channelSeq=%d sshSeq=%d", channelSeq, sshSeq)
	}
	if channelSeq >= sshSeq {
		t.Fatalf("expected channel-bus close (seq %d) to happen before sshClient.Close (seq %d)", channelSeq, sshSeq)
	}
}

// TestCloseSession_ChannelCascade_OneDirectional proves closing a single channel (simulated by
// the channel bus's internal bookkeeping) never cascades upward to affect the parent session or
// its sibling channels prematurely — the relationship is session->channels only, never the
// reverse. We assert this by showing the session stays open (registry entry present, sshClient
// never closed) when nothing calls SessionLifecycleService.CloseSession, even though the fake
// channel-close hook fires independently.
func TestCloseSession_ChannelCascade_OneDirectional(t *testing.T) {
	seq := &seqRecorder{}
	lifecycle, registry := channelCloseTestLifecycle(t)
	bus := newFakeChannelBus(seq)
	lifecycle.SetChannelBus(bus)

	sshClient := &fakeCloseSSHClient{seq: seq}
	const sessionID = "sess-onedir-1"
	const siblingSessionID = "sess-onedir-2"
	putTestSessionEntry(registry, sessionID, sshClient)

	siblingSSHClient := &fakeCloseSSHClient{seq: seq}
	putTestSessionEntry(registry, siblingSessionID, siblingSSHClient)

	// Simulate an individual channel on siblingSessionID closing (e.g. via channel.close RPC),
	// independent of any session-level action.
	bus.CloseSession(siblingSessionID)

	if _, ok := registry.Get(sessionID); !ok {
		t.Fatalf("closing a single channel must not remove the unrelated parent session %s from the registry", sessionID)
	}
	if _, ok := registry.Get(siblingSessionID); !ok {
		t.Fatalf("closing a single channel must not remove its own parent session %s from the registry", siblingSessionID)
	}
	if sshClient.closeCalls != 0 {
		t.Fatalf("closing a channel must never close an unrelated session's ssh client, got %d calls", sshClient.closeCalls)
	}
	if siblingSSHClient.closeCalls != 0 {
		t.Fatalf("closing a single channel must not cascade upward and close its own session's ssh client, got %d calls", siblingSSHClient.closeCalls)
	}

	// Now actually close the sibling session and confirm the cascade only fires once more
	// (proving the earlier single-channel close call didn't already tear the session down).
	if err := lifecycle.CloseSession(siblingSessionID); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}
	if bus.callsFor(siblingSessionID) != 2 {
		t.Fatalf("expected 2 CloseSession calls (1 simulated channel close + 1 real session close), got %d", bus.callsFor(siblingSessionID))
	}
	if siblingSSHClient.closeCalls != 1 {
		t.Fatalf("expected sshClient.Close called exactly once after real session close, got %d", siblingSSHClient.closeCalls)
	}
}

// TestCloseSession_ChannelCascade_ViaSessionManagerFacade proves the crash-recovery path — which
// funnels through the same SessionManager/SessionLifecycleService.CloseSession used by every
// other teardown path (plain disconnect, supervisor give-up, UI-driven close) — also cascades
// channel close. There is only one CloseSession implementation; this test proves it's reachable
// (and behaves identically) via the composition-root facade a crash-driven close would use,
// rather than re-testing the ordering logic already covered above.
func TestCloseSession_ChannelCascade_ViaSessionManagerFacade(t *testing.T) {
	seq := &seqRecorder{}
	registry := NewSessionRegistry()
	sm := NewSessionManager(SessionManagerConfig{})
	sm.registry = registry
	lifecycle := NewSessionLifecycleService(SessionLifecycleConfig{Registry: registry})
	sm.lifecycle = lifecycle

	bus := newFakeChannelBus(seq)
	sm.SetChannelBus(bus)

	sshClient := &fakeCloseSSHClient{seq: seq}
	const sessionID = "sess-crash-1"
	putTestSessionEntry(registry, sessionID, sshClient)

	if err := sm.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	if bus.callsFor(sessionID) != 1 {
		t.Fatalf("expected channel bus CloseSession invoked via the crash-reachable facade, got %d calls", bus.callsFor(sessionID))
	}
	if bus.closedAtFor(sessionID) >= sshClient.closeAt {
		t.Fatalf("expected channel cascade before sshClient.Close on the crash-reachable path too")
	}

	// Give any stray goroutine a moment in case a future regression makes this path async;
	// CloseSession must be synchronous, so this should never be needed to observe the result.
	time.Sleep(0)
}
