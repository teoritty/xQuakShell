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
	backend := NewChannelEmbedBackend("com.test", false, sink, nil)

	err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main")
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied (no session.embed capability)", err)
	}
}

func TestChannelEmbedBackend_Authorize_NotOwningSession_Denied(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.other-plugin"
	backend := NewChannelEmbedBackend("com.test", true, sink, nil)

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
	backend := NewChannelEmbedBackend("com.test", true, sink, nil)

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
	backend := NewChannelEmbedBackend("com.test", true, sink, nil)

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
