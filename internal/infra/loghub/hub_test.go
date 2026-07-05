package loghub

import (
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
