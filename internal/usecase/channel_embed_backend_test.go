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
// an external test observation point, the exact externally-visible behavior of Stage 5's real
// policyDropOldestUnsent branch in ipc/channel.go: send immediately while credit remains,
// otherwise stage the newest frame and evict the oldest staged one once at capacity. This
// backend never reimplements that policy itself — it only calls Send — so this fake exists to
// prove Wire's real code path reaches Send under exhaustion, not to duplicate channel.go's
// eviction logic.
type fakeEmbedDataPath struct {
	mu sync.Mutex

	inbound chan []byte
	closed  chan struct{}
	once    sync.Once

	credit     int
	stagingCap int
	staged     [][]byte
	sent       [][]byte
}

func newFakeEmbedDataPath(credit, stagingCap int) *fakeEmbedDataPath {
	return &fakeEmbedDataPath{
		inbound:    make(chan []byte, 16),
		closed:     make(chan struct{}),
		credit:     credit,
		stagingCap: stagingCap,
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

func (f *fakeEmbedDataPath) Send(_ context.Context, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	buf := append([]byte(nil), payload...)
	if f.credit > 0 {
		f.credit--
		f.sent = append(f.sent, buf)
		return nil
	}
	if f.stagingCap < 1 {
		f.stagingCap = 1
	}
	if len(f.staged) >= f.stagingCap {
		f.staged = f.staged[1:]
	}
	f.staged = append(f.staged, buf)
	return nil
}

func (f *fakeEmbedDataPath) WaitForCapacity(_ context.Context) error { return nil }

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

func (f *fakeEmbedDataPath) stagedFrames() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.staged))
	copy(out, f.staged)
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

	data := newFakeEmbedDataPath(100, 4)
	frame := []byte{0x00, 0xFF, 'a', 'b', 0x7F}
	data.inbound <- frame

	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeEmbedStream, ParentSessionID: "sess-1", Data: data}
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

// TestChannelEmbedBackend_Wire_OverflowDropsOldest drives the embed surface's outbound
// subscription through Wire's real outbound goroutine with credit exhausted, and confirms the
// staged frames end up latest-wins (oldest evicted first) — reachable purely by calling Wire,
// with no test-side shortcut around the backend's own Send-calling code path.
func TestChannelEmbedBackend_Wire_OverflowDropsOldest(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"
	sink.subCh = make(chan []byte, 8)
	backend := NewChannelEmbedBackend("com.test", true, sink, nil)

	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main"); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeEmbedDataPath(0, 2) // no credit at all: every Send must land in staging
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeEmbedStream, ParentSessionID: "sess-1", Data: data}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	sink.subCh <- []byte("frame-a")
	sink.subCh <- []byte("frame-b")
	sink.subCh <- []byte("frame-c")
	sink.subCh <- []byte("frame-d")

	deadline := time.After(2 * time.Second)
	for {
		if len(data.stagedFrames()) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for staging to settle, got %d staged", len(data.stagedFrames()))
		case <-time.After(10 * time.Millisecond):
		}
	}

	staged := data.stagedFrames()
	if string(staged[0]) != "frame-c" || string(staged[1]) != "frame-d" {
		t.Fatalf("staged = %q, want [frame-c frame-d] (oldest two must have been evicted, latest-wins)", staged)
	}
	if sent := data.sentFrames(); len(sent) != 0 {
		t.Fatalf("sentFrames = %q, want none (credit was exhausted throughout)", sent)
	}

	backend.CloseRemote()
	if atomic.LoadInt32(&sink.unsubscribed) != 1 {
		t.Fatalf("unsubscribed count = %d, want 1", sink.unsubscribed)
	}
}
