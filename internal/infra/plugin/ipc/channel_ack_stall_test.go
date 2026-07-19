package ipc

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// captureSlog redirects slog to a buffer for the duration of a test.
func captureSlog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newStallTestChannel(t *testing.T, after time.Duration) *channel {
	t.Helper()
	ch := newFlowChannel(1, domainplugin.PurposeExec, 1, domainplugin.DefaultChannelThroughputKbps,
		nil, func([]byte) error { return nil })
	ch.ackStallAfter = after
	return ch
}

// TestAckStallIsReportedWhenBackendNeverAcks: a forgotten Ack stalls the channel only after the
// first window drains, and does so silently. Explicit Ack is the right contract — it keeps Recv
// to one job — but the price is that it can be forgotten, so the failure has to be loud.
func TestAckStallIsReportedWhenBackendNeverAcks(t *testing.T) {
	logs := captureSlog(t)
	ch := newStallTestChannel(t, 20*time.Millisecond)

	// Spend the whole (1-frame) inbound window; nothing ever Acks.
	if err := ch.deliver(domainplugin.FrameKindBinary, []byte("frame")); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for !strings.Contains(logs.String(), "channel stalled") {
		select {
		case <-deadline:
			t.Fatal("inbound window sat exhausted and un-Acked with nothing logged; a backend author would see only a channel that stops after N frames")
		case <-time.After(5 * time.Millisecond):
		}
	}

	out := logs.String()
	if !strings.Contains(out, "channelId=1") || !strings.Contains(out, "purpose=exec") {
		t.Fatalf("stall report must name the channel and purpose to be actionable, got: %s", out)
	}
}

// TestAckStallIsNotReportedWhenBackendAcks: the watchdog must stay quiet on a healthy channel,
// or it becomes noise everyone learns to ignore.
func TestAckStallIsNotReportedWhenBackendAcks(t *testing.T) {
	logs := captureSlog(t)
	ch := newStallTestChannel(t, 20*time.Millisecond)

	if err := ch.deliver(domainplugin.FrameKindBinary, []byte("frame")); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	ch.grantInbound(1) // what Ack does

	time.Sleep(80 * time.Millisecond)

	if strings.Contains(logs.String(), "channel stalled") {
		t.Fatalf("stall reported on a channel that was Acked: %s", logs.String())
	}
}

// TestAckStallIsNotReportedAfterClose: a closed channel is not a stalled one — nobody is waiting
// on its credit.
func TestAckStallIsNotReportedAfterClose(t *testing.T) {
	logs := captureSlog(t)
	ch := newStallTestChannel(t, 20*time.Millisecond)

	if err := ch.deliver(domainplugin.FrameKindBinary, []byte("frame")); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	ch.Close()

	time.Sleep(80 * time.Millisecond)

	if strings.Contains(logs.String(), "channel stalled") {
		t.Fatalf("stall reported for a closed channel: %s", logs.String())
	}
}
