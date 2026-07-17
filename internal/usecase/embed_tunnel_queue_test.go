package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// attachStalledTunnel registers a session, opens a tunnel and attaches a browser end whose
// consumer never reads. The returned stream stands in for a browser that has stopped draining.
func attachStalledTunnel(t *testing.T, svc *EmbedTunnelService, sessionID string) domain.EmbedTunnelStream {
	t.Helper()
	ctx := context.Background()
	desc, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: sessionID,
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSessionActive(sessionID, true)
	if err := svc.OpenTunnel(sessionID, "main"); err != nil {
		t.Fatal(err)
	}
	stream, _, err := svc.AttachWebSocket(extractEmbedToken(desc.UIUrl), "main")
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

// TestStalledBrowserHoldsAtMostACreditWindowOfFrames is D10's memory assertion (§5 B9 "Done when").
//
// The credit window is the only thing that may hold host memory for a tunnel. If the send queue
// behind the sink is deeper than the window, the plugin's window reopens on *enqueue* and the queue
// silently absorbs frames the browser has not taken: at 256 frames x 64 KiB that is 16 MiB per
// tunnel, 33x what D2 claims, in memory the plugin's Job Object does not bound.
//
// This test measures the queue the only way that is falsifiable from the sink's side: it stalls the
// browser and counts the bytes the host accepts before it refuses. A regression that "optimises"
// the buffer back up fails here, because accepting more frames is exactly what a deeper queue does.
func TestStalledBrowserHoldsAtMostACreditWindowOfFrames(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()
	attachStalledTunnel(t, svc, "sess-stall") // nobody ever reads Send()

	frame := make([]byte, domain.MaxTunnelFrameSize)
	accepted := 0
	// Push far past any plausible window; stop at the first refusal.
	for i := 0; i < 4096; i++ {
		if err := svc.RouteTunnelFrameFromPlugin(ctx, "sess-stall", "main", frame); err != nil {
			if cause, ok := EmbedRefusalCauseOf(err); !ok || cause != EmbedRefusedWSBufferFull {
				t.Fatalf("frame %d: expected ws-buffer-full, got %v", i, err)
			}
			break
		}
		accepted++
	}
	if accepted == 0 {
		t.Fatal("host refused every frame; the queue must still admit a window")
	}

	window := domainplugin.InitialCredit(domainplugin.PurposeEmbedStream)
	// "Within the credit window's order of magnitude" (B9): the in-flight frame the pump is
	// writing is not in the queue, so allow the window itself plus a small constant, never a
	// multiple of it.
	limit := window + 2
	held := accepted * domain.MaxTunnelFrameSize
	if accepted > limit {
		t.Fatalf("host held %d frames (%d KiB) for a stalled browser; the credit window is %d frames (%d KiB). "+
			"The send queue is hiding more than a window: D10/§3.14 memory bomb.",
			accepted, held/1024, window, window*domain.MaxTunnelFrameSize/1024)
	}
}

// TestStalledThenResumingBrowserLosesNoFrames proves the shrunken queue refuses, and never drops
// (§7.3): embed-stream carries state transitions in both directions, so a caller that waits on
// ws-buffer-full -- which is what the embed backend's pump does since B4 -- must still see every
// frame, in order. Drives InitialCredit x 3 so the window has to reopen (§7.2).
func TestStalledThenResumingBrowserLosesNoFrames(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()
	stream := attachStalledTunnel(t, svc, "sess-resume")

	total := domainplugin.InitialCredit(domainplugin.PurposeEmbedStream) * 3
	got := make(chan []byte, total)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			select {
			case data := <-stream.Send():
				got <- data
			case <-time.After(5 * time.Second):
				return
			}
		}
	}()

	// The producer waits on ws-buffer-full instead of dropping, exactly as deliverFrame does.
	for i := 0; i < total; i++ {
		payload := []byte(fmt.Sprintf("frame-%d", i))
		for {
			err := svc.RouteTunnelFrameFromPlugin(ctx, "sess-resume", "main", payload)
			if err == nil {
				break
			}
			if cause, ok := EmbedRefusalCauseOf(err); !ok || cause != EmbedRefusedWSBufferFull {
				t.Fatalf("frame %d: unexpected refusal %v", i, err)
			}
			time.Sleep(time.Millisecond)
		}
	}
	<-done
	if len(got) != total {
		t.Fatalf("consumer received %d of %d frames; frames were dropped", len(got), total)
	}
	for i := 0; i < total; i++ {
		want := fmt.Sprintf("frame-%d", i)
		if string(<-got) != want {
			t.Fatalf("frame %d out of order", i)
		}
	}
}

// TestHealthyBrowserDoesNotStall is D10's own condition on preferring option 1 ("prefer shrinking
// conn.send unless it measurably stalls a healthy browser"). A consumer that drains promptly must
// never see a refusal, whatever the queue depth.
func TestHealthyBrowserDoesNotStall(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()
	stream := attachStalledTunnel(t, svc, "sess-healthy")

	const total = 2000
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			<-stream.Send()
		}
	}()

	frame := make([]byte, domain.MaxTunnelFrameSize)
	refusals := 0
	start := time.Now()
	for i := 0; i < total; i++ {
		for {
			err := svc.RouteTunnelFrameFromPlugin(ctx, "sess-healthy", "main", frame)
			if err == nil {
				break
			}
			if cause, ok := EmbedRefusalCauseOf(err); !ok || cause != EmbedRefusedWSBufferFull {
				t.Fatalf("frame %d: unexpected refusal %v", i, err)
			}
			refusals++
		}
	}
	<-done
	elapsed := time.Since(start)
	t.Logf("healthy browser: %d frames (%d MiB) in %v, %d transient ws-buffer-full retries",
		total, total*domain.MaxTunnelFrameSize/(1024*1024), elapsed, refusals)
	if elapsed > 5*time.Second {
		t.Fatalf("healthy browser stalled: %v for %d frames", elapsed, total)
	}
}
