package usecase

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// fakeEmbedSink is a test double for EmbedFrameSink: RouteTunnelFrameFromPlugin records the raw
// bytes the backend hands it (proving no base64/JSON wrapping happens along the way),
// PluginIDForSession simulates the ADR-008 embed registration ownership map, and
// SubscribeOutbound hands back a test-controlled channel simulating the embed surface's
// host->plugin byte stream.
type fakeEmbedSink struct {
	mu sync.Mutex

	owners     map[string]string
	fromPlugin [][]byte
	fromErr    error

	subCh        chan []byte
	unsubscribed int32

	// refuseWith, when non-nil, is consulted before accepting a frame: a non-nil return refuses it,
	// standing in for EmbedTunnelService's classified refusals.
	refuseWith func() error
}

func newFakeEmbedSink() *fakeEmbedSink {
	return &fakeEmbedSink{owners: make(map[string]string)}
}

func (s *fakeEmbedSink) RouteTunnelFrameFromPlugin(_ context.Context, _ string, _ string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fromErr != nil {
		return s.fromErr
	}
	if s.refuseWith != nil {
		if err := s.refuseWith(); err != nil {
			return err
		}
	}
	s.fromPlugin = append(s.fromPlugin, append([]byte(nil), data...))
	return nil
}

func (s *fakeEmbedSink) PluginIDForSession(sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.owners[sessionID]
	return id, ok
}

func (s *fakeEmbedSink) SubscribeOutbound(_ string, _ string) (<-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subCh == nil {
		return nil, func() {}
	}
	return s.subCh, func() { atomic.AddInt32(&s.unsubscribed, 1) }
}

func (s *fakeEmbedSink) framesFromPlugin() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.fromPlugin))
	copy(out, s.fromPlugin)
	return out
}

// fakeEmbedDataPath is a domainplugin.ChannelDataPath test double whose Send mirrors, purely as
// an external test observation point, the externally-visible behavior of the real channel in
// ipc/channel.go: send while credit remains, block once it is exhausted, and never discard a
// frame. This backend never implements that policy itself — it only calls Send — so the fake
// exists to prove Wire's real code path reaches Send under exhaustion, not to duplicate
// channel.go's logic.
type fakeEmbedDataPath struct {
	mu   sync.Mutex
	acks atomic.Int64

	inbound chan []byte
	closed  chan struct{}
	once    sync.Once

	// credit holds one token per sendable frame, mirroring the real outbound credit window.
	credit chan struct{}
	sent   [][]byte
}

func newFakeEmbedDataPath(credit int) *fakeEmbedDataPath {
	f := &fakeEmbedDataPath{
		inbound: make(chan []byte, 16),
		closed:  make(chan struct{}),
		credit:  make(chan struct{}, 1024),
	}
	f.grantCredit(credit)
	return f
}

// grantCredit makes n more frames sendable, standing in for an inbound kind=0x03 frame.
func (f *fakeEmbedDataPath) grantCredit(n int) {
	for i := 0; i < n; i++ {
		f.credit <- struct{}{}
	}
}

func (f *fakeEmbedDataPath) Recv() ([]byte, bool) {
	select {
	case b, ok := <-f.inbound:
		return b, ok
	case <-f.closed:
		return nil, false
	}
}

func (f *fakeEmbedDataPath) Send(ctx context.Context, payload []byte) error {
	buf := append([]byte(nil), payload...)
	select {
	case <-f.credit:
	case <-ctx.Done():
		return ctx.Err()
	case <-f.closed:
		return context.Canceled
	}
	f.mu.Lock()
	f.sent = append(f.sent, buf)
	f.mu.Unlock()
	return nil
}

func (f *fakeEmbedDataPath) WaitForCapacity(_ context.Context) error { return nil }

func (f *fakeEmbedDataPath) Ack(_ context.Context) error {
	f.acks.Add(1)
	return nil
}

func (f *fakeEmbedDataPath) ackCount() int64 { return f.acks.Load() }

func (f *fakeEmbedDataPath) Close() error {
	f.closeChannel()
	return nil
}

func (f *fakeEmbedDataPath) closeChannel() {
	f.once.Do(func() { close(f.closed) })
}

func (f *fakeEmbedDataPath) sentFrames() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.sent))
	copy(out, f.sent)
	return out
}

func TestChannelEmbedBackend_Authorize_NoEmbedCapability_Denied(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"
	backend := NewChannelEmbedBackend("com.test", false, sink, nil, nil)

	err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main")
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied (no session.embed capability)", err)
	}
}

func TestChannelEmbedBackend_Authorize_NotOwningSession_Denied(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.other-plugin"
	backend := NewChannelEmbedBackend("com.test", true, sink, nil, nil)

	err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main")
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied (session owned by another plugin)", err)
	}
}

// TestChannelEmbedBackend_Wire_FramesFlowRawToEmbedSink is the crux raw-passthrough test:
// binary bytes (including a NUL and non-UTF8 byte, which a base64/JSON path would mangle without
// explicit encoding) sent by the plugin must arrive at the embed sink byte-for-byte.
func TestChannelEmbedBackend_Wire_FramesFlowRawToEmbedSink(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"
	backend := NewChannelEmbedBackend("com.test", true, sink, nil, nil)

	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main"); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeEmbedDataPath(100)
	frame := []byte{0x00, 0xFF, 'a', 'b', 0x7F}
	data.inbound <- frame

	handle := mustChannelHandle(t, 1, "com.test", domainplugin.PurposeEmbedStream, "sess-1", "", data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	deadline := time.After(2 * time.Second)
	for {
		if len(sink.framesFromPlugin()) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for frame to reach embed sink")
		case <-time.After(10 * time.Millisecond):
		}
	}

	got := sink.framesFromPlugin()[0]
	if len(got) != len(frame) {
		t.Fatalf("frame len = %d, want %d (bytes must pass through unmodified, not re-encoded)", len(got), len(frame))
	}
	for i := range frame {
		if got[i] != frame[i] {
			t.Fatalf("frame[%d] = %x, want %x", i, got[i], frame[i])
		}
	}
}

// TestChannelEmbedBackend_Wire_OverflowLosesNoInputFrame drives the embed surface's outbound
// subscription — the browser's control input — through Wire's real outbound goroutine with
// credit exhausted, and confirms every frame eventually reaches the plugin, in order.
//
// This replaces an assertion that the oldest frames were evicted. That policy was written for
// video; this direction carries input, where a dropped frame is a dropped state transition: an
// evicted key-up leaves the key held down on the remote machine. Late is acceptable here, lost
// is not.
func TestChannelEmbedBackend_Wire_OverflowLosesNoInputFrame(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"
	sink.subCh = make(chan []byte, 8)
	backend := NewChannelEmbedBackend("com.test", true, sink, nil, nil)

	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main"); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeEmbedDataPath(0) // no credit at all: every Send must wait
	handle := mustChannelHandle(t, 1, "com.test", domainplugin.PurposeEmbedStream, "sess-1", "", data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	input := []string{"frame-a", "frame-b", "frame-c", "frame-d"}
	for _, f := range input {
		sink.subCh <- []byte(f)
	}

	// Nothing may reach the wire while the window is shut.
	time.Sleep(100 * time.Millisecond)
	if sent := data.sentFrames(); len(sent) != 0 {
		t.Fatalf("sentFrames = %q, want none while credit is exhausted", sent)
	}

	// Open the window; every frame must arrive, none evicted.
	data.grantCredit(len(input))

	deadline := time.After(2 * time.Second)
	for len(data.sentFrames()) < len(input) {
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d of %d input frames reached the plugin (%q) — frames were dropped",
				len(data.sentFrames()), len(input), data.sentFrames())
		case <-time.After(10 * time.Millisecond):
		}
	}

	sent := data.sentFrames()
	for i, want := range input {
		if string(sent[i]) != want {
			t.Fatalf("sent[%d] = %q, want %q (input must arrive in order, none lost)", i, sent[i], want)
		}
	}

	backend.CloseRemote()
	if atomic.LoadInt32(&sink.unsubscribed) != 1 {
		t.Fatalf("unsubscribed count = %d, want 1", sink.unsubscribed)
	}
}

// embedInboundFrames feeds n numbered frames into the pump's Recv side and returns what the sink
// must end up holding, in order. It drives past InitialCredit x 3 (the embed-stream window is 8),
// because a test that moves 8 frames never proves the window reopens.
func embedInboundFrames(data *fakeEmbedDataPath, n int) []string {
	want := make([]string, 0, n)
	for i := 0; i < n; i++ {
		want = append(want, string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	go func() {
		for _, f := range want {
			data.inbound <- []byte(f)
		}
	}()
	return want
}

const embedTestFrames = 24 // InitialCredit(8) x 3, per the readiness doc's 7.2

func waitForFrames(t *testing.T, sink *fakeEmbedSink, want []string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for len(sink.framesFromPlugin()) < len(want) {
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d of %d frames reached the embed surface", len(sink.framesFromPlugin()), len(want))
		case <-time.After(5 * time.Millisecond):
		}
	}
	got := sink.framesFromPlugin()
	for i, w := range want {
		if string(got[i]) != w {
			t.Fatalf("frame[%d] = %q, want %q (frames must arrive in order, none lost)", i, got[i], w)
		}
	}
}

// newWiredEmbedBackend authorizes and wires a backend with a test-scale ceiling. The ceiling and
// retry interval are the SAME fields production uses, set by the SAME constructor — lowered here
// only so CI does not sleep for two minutes.
func newWiredEmbedBackend(t *testing.T, sink *fakeEmbedSink, data *fakeEmbedDataPath, ceiling time.Duration, notify ChannelCloseNotifier) *ChannelEmbedBackend {
	t.Helper()
	sink.owners["sess-1"] = "com.test"
	backend := NewChannelEmbedBackend("com.test", true, sink, nil, notify)
	backend.ackCeiling = ceiling
	backend.retryInterval = 2 * time.Millisecond
	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	handle := mustChannelHandle(t, 7, "com.test", domainplugin.PurposeEmbedStream, "sess-1", "", data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	t.Cleanup(func() { backend.CloseRemote() })
	return backend
}

// TestChannelEmbedBackend_BrieflyStalledConsumerWaitsAndResumes is D4's normal case: a browser
// that is momentarily behind must not cost the plugin its channel. The frame in hand is not
// dropped and not Acked — so the plugin's window closes and the backpressure reaches the VNC
// server — and when the consumer drains, every frame continues in order.
func TestChannelEmbedBackend_BrieflyStalledConsumerWaitsAndResumes(t *testing.T) {
	sink := newFakeEmbedSink()
	var stalled atomic.Bool
	stalled.Store(true)
	sink.refuseWith = func() error {
		if stalled.Load() {
			return newEmbedRefusal(EmbedRefusedWSBufferFull, domainplugin.ErrTerminalBackpressure)
		}
		return nil
	}

	data := newFakeEmbedDataPath(0)
	backend := newWiredEmbedBackend(t, sink, data, 5*time.Second, nil)
	want := embedInboundFrames(data, embedTestFrames)

	// While the consumer is stalled: nothing delivered, and crucially nothing Acked. Acking here
	// would reopen the plugin's window against a consumer that never took the frame.
	time.Sleep(80 * time.Millisecond)
	if n := len(sink.framesFromPlugin()); n != 0 {
		t.Fatalf("frames delivered = %d, want 0 while the consumer is stalled", n)
	}
	if acks := data.ackCount(); acks != 0 {
		t.Fatalf("acks = %d, want 0 while the frame has not reached the consumer", acks)
	}
	if backend.isClosed() {
		t.Fatal("channel was torn down by a brief consumer stall — this is the B4 defect")
	}

	stalled.Store(false)
	waitForFrames(t, sink, want)
	if acks := data.ackCount(); acks < int64(len(want)) {
		t.Fatalf("acks = %d, want >= %d once every frame was accepted", acks, len(want))
	}
}

// TestChannelEmbedBackend_StallPastCeilingClosesWithReason is D4's other half: waiting is not
// forever. An active tab whose buffer has not drained for the whole ceiling is gone, and the
// channel must die with a reason the plugin can read, not wedge silently.
func TestChannelEmbedBackend_StallPastCeilingClosesWithReason(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.refuseWith = func() error {
		return newEmbedRefusal(EmbedRefusedWSBufferFull, domainplugin.ErrTerminalBackpressure)
	}

	type closeCall struct {
		channelID       uint32
		reason, message string
	}
	closes := make(chan closeCall, 4)
	data := newFakeEmbedDataPath(0)
	backend := newWiredEmbedBackend(t, sink, data, 100*time.Millisecond, func(channelID uint32, reason, message string) {
		closes <- closeCall{channelID, reason, message}
	})
	embedInboundFrames(data, embedTestFrames)

	select {
	case got := <-closes:
		if got.reason != string(EmbedRefusedWSBufferFull) {
			t.Fatalf("close reason = %q, want %q", got.reason, EmbedRefusedWSBufferFull)
		}
		if got.channelID != 7 {
			t.Fatalf("close channelID = %d, want 7", got.channelID)
		}
		if got.message == "" {
			t.Fatal("close message is empty — the plugin has no other way to learn why it died")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer stalled past the ceiling but the channel was never closed with a reason")
	}
	if !backend.isClosed() {
		t.Fatal("ceiling fired but the backend was not closed")
	}
	if acks := data.ackCount(); acks != 0 {
		t.Fatalf("acks = %d, want 0 — no frame ever reached the consumer", acks)
	}
}

// TestChannelEmbedBackend_InactiveTabPausesTheCeiling is D5, and it is the test that fails if D4
// is implemented naively: tab-inactive shares a sentinel with ws-buffer-full, so charging the
// ceiling against it kills the VNC session of every user who looks at another tab for two minutes
// — the exact bug B4 exists to fix, through the front door. A backgrounded tab PAUSES the
// ceiling; it does not spend it.
func TestChannelEmbedBackend_InactiveTabPausesTheCeiling(t *testing.T) {
	sink := newFakeEmbedSink()
	var inactive atomic.Bool
	inactive.Store(true)
	sink.refuseWith = func() error {
		if inactive.Load() {
			return newEmbedRefusal(EmbedRefusedTabInactive, domainplugin.ErrTerminalBackpressure)
		}
		return nil
	}

	closes := make(chan string, 4)
	data := newFakeEmbedDataPath(0)
	const ceiling = 50 * time.Millisecond
	backend := newWiredEmbedBackend(t, sink, data, ceiling, func(_ uint32, reason, _ string) {
		closes <- reason
	})
	want := embedInboundFrames(data, embedTestFrames)

	// Backgrounded for many times the ceiling: the user is simply looking elsewhere.
	select {
	case reason := <-closes:
		t.Fatalf("channel closed with %q while the tab was merely backgrounded — the ceiling was spent, not paused", reason)
	case <-time.After(10 * ceiling):
	}
	if backend.isClosed() {
		t.Fatal("backend closed against a backgrounded tab")
	}

	// The user comes back.
	inactive.Store(false)
	waitForFrames(t, sink, want)
	select {
	case reason := <-closes:
		t.Fatalf("channel closed with %q after the tab returned", reason)
	default:
	}
}
