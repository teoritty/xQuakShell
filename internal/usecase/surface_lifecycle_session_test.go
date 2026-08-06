package usecase

import (
	"sync"
	"testing"
)

// fakeSurfaceCloser is the surface counterpart of fakeChannelBus
// (session_lifecycle_channel_close_test.go): it records the sequence number at which the cascade
// reached it, so ordering against the channel cascade and the ssh close can be asserted rather
// than merely presence.
type fakeSurfaceCloser struct {
	seq *seqRecorder

	mu       sync.Mutex
	closedAt map[string]int
	calls    map[string]int
}

func newFakeSurfaceCloser(seq *seqRecorder) *fakeSurfaceCloser {
	return &fakeSurfaceCloser{seq: seq, closedAt: make(map[string]int), calls: make(map[string]int)}
}

func (c *fakeSurfaceCloser) CloseSurfacesForSession(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[sessionID]++
	c.closedAt[sessionID] = c.seq.next_()
}

func (c *fakeSurfaceCloser) callsFor(sessionID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[sessionID]
}

func (c *fakeSurfaceCloser) closedAtFor(sessionID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closedAt[sessionID]
}

// A surface must not outlive the session whose authorization it borrowed (ADR-015 §1), and it is
// usually fed by a channel on that same session — so the producer is torn down first and the tab
// is removed with nothing still trying to write into it. Both run before the ssh client closes,
// extending the invariant TestCloseSession_ChannelCascade_BeforeSSHClientClose already pins.
func TestCloseSessionClosesChannelsThenSurfacesBeforeSSHClose(t *testing.T) {
	seq := &seqRecorder{}
	lifecycle, registry := channelCloseTestLifecycle(t)
	bus := newFakeChannelBus(seq)
	surfaces := newFakeSurfaceCloser(seq)
	lifecycle.SetChannelBus(bus)
	lifecycle.SetSurfaces(surfaces)

	sshClient := &fakeCloseSSHClient{seq: seq}
	const sessionID = "sess-surface-cascade"
	putTestSessionEntry(registry, sessionID, sshClient)

	if err := lifecycle.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	if surfaces.callsFor(sessionID) != 1 {
		t.Fatalf("surface cascade called %d times, want 1", surfaces.callsFor(sessionID))
	}
	channelSeq := bus.closedAtFor(sessionID)
	surfaceSeq := surfaces.closedAtFor(sessionID)
	sshSeq := sshClient.closeAt
	if channelSeq == 0 || surfaceSeq == 0 || sshSeq == 0 {
		t.Fatalf("expected all three teardown steps to run, got channels=%d surfaces=%d ssh=%d",
			channelSeq, surfaceSeq, sshSeq)
	}
	if !(channelSeq < surfaceSeq && surfaceSeq < sshSeq) {
		t.Fatalf("teardown order was channels=%d surfaces=%d ssh=%d, want channels < surfaces < ssh",
			channelSeq, surfaceSeq, sshSeq)
	}
}

// A session with no surface closer wired must close exactly as before. The closer is late-bound,
// so every test and every path that predates it keeps working.
func TestCloseSessionWithoutSurfaceCloserStillCloses(t *testing.T) {
	seq := &seqRecorder{}
	lifecycle, registry := channelCloseTestLifecycle(t)
	sshClient := &fakeCloseSSHClient{seq: seq}
	putTestSessionEntry(registry, "sess-no-surfaces", sshClient)

	if err := lifecycle.CloseSession("sess-no-surfaces"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if sshClient.closeCalls != 1 {
		t.Fatalf("sshClient.Close calls = %d, want 1", sshClient.closeCalls)
	}
}
