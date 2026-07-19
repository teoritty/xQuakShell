package loghub

import (
	"log/slog"
	"strings"
	"testing"

	"ssh-client/internal/domain"
)

func TestHubRingBuffer(t *testing.T) {
	h := NewHub(3)
	for i := 0; i < 5; i++ {
		h.Publish(domain.DebugLogEntry{Message: string(rune('a' + i))})
	}
	snap := h.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap))
	}
	if snap[0].Message != "c" || snap[2].Message != "e" {
		t.Fatalf("unexpected ring order: %+v", snap)
	}
}

func TestHubSubscribeLive(t *testing.T) {
	h := NewHub(10)
	id, backlog, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	if len(backlog) != 0 {
		t.Fatalf("expected empty backlog")
	}
	h.Publish(domain.DebugLogEntry{Message: "live"})
	select {
	case e := <-ch:
		if e.Message != "live" {
			t.Fatalf("unexpected message %q", e.Message)
		}
	default:
		t.Fatal("expected live entry")
	}
}

// TestHubDropMarker verifies that when a subscriber falls behind, entries are
// counted and surfaced as a synthetic drop marker rather than lost silently,
// and that Publish never blocks on a full subscriber.
func TestHubDropMarker(t *testing.T) {
	h := NewHub(1000)
	// Tiny buffer so the subscriber overflows immediately.
	id, _, ch := h.Subscribe(2)
	defer h.Unsubscribe(id)

	// Publish far more than the buffer holds without draining. This must not
	// block even though the channel is full.
	const total = 50
	for range total {
		h.Publish(domain.DebugLogEntry{Message: "flood"})
	}

	// Drain and look for at least one drop marker reporting lost entries.
	var sawMarker bool
	for {
		select {
		case e := <-ch:
			if e.Source == dropMarkerSource && strings.Contains(e.Message, "dropped") {
				sawMarker = true
			}
			continue
		default:
		}
		break
	}
	// One more publish flushes any pending marker accumulated after the drain.
	h.Publish(domain.DebugLogEntry{Message: "after"})
	for {
		select {
		case e := <-ch:
			if e.Source == dropMarkerSource {
				sawMarker = true
			}
			continue
		default:
		}
		break
	}
	if !sawMarker {
		t.Fatal("expected a drop marker after overflowing the subscriber")
	}
}

func TestLevelGating(t *testing.T) {
	SetLevel(slog.LevelWarn)
	defer SetLevel(slog.LevelDebug)
	if Enabled(slog.LevelInfo) {
		t.Fatal("info should be gated out at warn floor")
	}
	if !Enabled(slog.LevelError) {
		t.Fatal("error should pass at warn floor")
	}
	if ParseLevel("warn") != slog.LevelWarn || ParseLevel("") != slog.LevelDebug {
		t.Fatal("ParseLevel mismatch")
	}
}
