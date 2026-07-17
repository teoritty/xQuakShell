package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// Multi-tunnel is a declared field (EmbedRegistration.TunnelIDs) that every other embed test
// exercises with exactly one id ("main"). These tests are the missing half: the routing rules that
// only exist once a session has more than one tunnel, which is the entire point of raising
// MaxTunnelsPerSession to 8 for SPICE/RDP.
//
// The failure mode these guard is not an error. A frame for "clipboard" arriving on "display" is
// silent corruption: RDP decodes it as pixels and nothing anywhere logs a thing.

// multiTunnelIDs is a realistic RDP-shaped subset -- three DISTINCT ids, none of them "main", so a
// backend that silently defaults to embedDefaultTunnelID cannot pass by accident.
var multiTunnelIDs = []string{"display", "clipboard", "audio-out"}

// newMultiTunnelSession registers one embed session with the given tunnel ids, attaches a browser
// WebSocket to each, and returns the per-tunnel streams keyed by id.
func newMultiTunnelSession(t *testing.T, sessionID string, ids []string) (*EmbedTunnelService, map[string]domain.EmbedTunnelStream) {
	t.Helper()
	svc := newTestEmbedTunnelService()
	desc, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: sessionID,
		PluginID:  "com.test.rdp",
		UIEntry:   "ui/rdp.html",
		TunnelIDs: ids,
	})
	if err != nil {
		t.Fatalf("register %d tunnels: %v", len(ids), err)
	}
	svc.SetSessionActive(sessionID, true)

	token := tokenFromDescriptor(t, desc)
	streams := make(map[string]domain.EmbedTunnelStream, len(ids))
	for _, id := range ids {
		if err := svc.OpenTunnel(sessionID, id); err != nil {
			t.Fatalf("open tunnel %q: %v", id, err)
		}
		stream, _, err := svc.AttachWebSocket(token, id)
		if err != nil {
			t.Fatalf("attach %q: %v", id, err)
		}
		streams[id] = stream
	}
	return svc, streams
}

// tokenFromDescriptor recovers the minted token from the descriptor's tunnel URL, which is the only
// place Register surfaces it.
func tokenFromDescriptor(t *testing.T, desc domain.SessionEmbedDescriptor) string {
	t.Helper()
	// .../embed/s/<token>/tunnel/<tunnelID>
	parts := strings.Split(strings.Trim(desc.TunnelUrl, "/"), "/")
	for i, p := range parts {
		if p == "s" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	t.Fatalf("no token in tunnel url %q", desc.TunnelUrl)
	return ""
}

// TestEmbedTunnels_NoCrossTunnelDelivery drives three tunnels concurrently with payloads that name
// their own tunnel, and asserts every frame arrived on the tunnel it was addressed to.
//
// InitialCredit x 3 = 24 frames per tunnel (7.2): enough that a per-tunnel WebSocket queue
// (embedWSSendQueueDepth = InitialCredit) must wrap three times, so the test exercises the steady
// state rather than a single burst that fits in the buffer.
func TestEmbedTunnels_NoCrossTunnelDelivery(t *testing.T) {
	svc, streams := newMultiTunnelSession(t, "sess-multi", multiTunnelIDs)
	const perTunnel = 3 * 8 // InitialCredit(embed-stream) x 3

	// Consumers first: the queue is only InitialCredit deep and must NOT drop, so a producer with
	// no reader would simply refuse with ws-buffer-full rather than corrupt anything.
	var wg sync.WaitGroup
	received := make(map[string][]string, len(streams))
	var mu sync.Mutex
	for id, stream := range streams {
		wg.Add(1)
		go func(id string, stream domain.EmbedTunnelStream) {
			defer wg.Done()
			for i := 0; i < perTunnel; i++ {
				select {
				case frame := <-stream.Send():
					mu.Lock()
					received[id] = append(received[id], string(frame))
					mu.Unlock()
				case <-time.After(5 * time.Second):
					t.Errorf("tunnel %q: timed out after %d frames", id, i)
					return
				}
			}
		}(id, stream)
	}

	for _, id := range multiTunnelIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for i := 0; i < perTunnel; i++ {
				payload := []byte(fmt.Sprintf("%s#%d", id, i))
				if err := routeUntilAccepted(svc, "sess-multi", id, payload); err != nil {
					t.Errorf("tunnel %q frame %d: %v", id, i, err)
					return
				}
			}
		}(id)
	}
	wg.Wait()

	for _, id := range multiTunnelIDs {
		got := received[id]
		if len(got) != perTunnel {
			t.Fatalf("tunnel %q received %d frames, want %d", id, len(got), perTunnel)
		}
		for i, frame := range got {
			// Both halves matter: the prefix catches a frame that crossed tunnels, and the ordinal
			// catches reordering or duplication within one.
			if want := fmt.Sprintf("%s#%d", id, i); frame != want {
				t.Fatalf("tunnel %q frame %d = %q, want %q -- a frame addressed to another tunnel "+
					"landed here, which for RDP is silent corruption rather than an error",
					id, i, frame, want)
			}
		}
	}
}

// routeUntilAccepted retries the transient ws-buffer-full refusal, which is the queue doing its job
// rather than a failure. Any other refusal is returned.
func routeUntilAccepted(svc *EmbedTunnelService, sessionID, tunnelID string, payload []byte) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := svc.RouteTunnelFrameFromPlugin(context.Background(), sessionID, tunnelID, payload)
		if err == nil {
			return nil
		}
		if cause, ok := EmbedRefusalCauseOf(err); !ok || cause != EmbedRefusedWSBufferFull {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("still ws-buffer-full after 5s")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestEmbedTunnels_BackpressureIsPerTunnel stalls ONE tunnel's consumer and asserts the other two
// keep flowing untouched.
//
// A shared stall is D5's bug with more steps: if one slow channel's backpressure reached its
// siblings, an RDP session would freeze its display because nobody read the clipboard.
func TestEmbedTunnels_BackpressureIsPerTunnel(t *testing.T) {
	svc, streams := newMultiTunnelSession(t, "sess-stall", multiTunnelIDs)
	const stalled = "clipboard"

	// Fill the stalled tunnel's queue until it refuses. Nobody reads streams[stalled].Send().
	var refusal error
	for i := 0; i < 100; i++ {
		refusal = svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-stall", stalled, []byte("x"))
		if refusal != nil {
			break
		}
	}
	cause, ok := EmbedRefusalCauseOf(refusal)
	if !ok || cause != EmbedRefusedWSBufferFull {
		t.Fatalf("stalled tunnel refusal = %v (cause %q), want ws-buffer-full", refusal, cause)
	}

	// The siblings must be entirely unaffected: full flow, no refusal of any kind.
	for _, id := range multiTunnelIDs {
		if id == stalled {
			continue
		}
		const frames = 3 * 8
		done := make(chan struct{})
		go func(id string) {
			defer close(done)
			for i := 0; i < frames; i++ {
				select {
				case <-streams[id].Send():
				case <-time.After(5 * time.Second):
					t.Errorf("tunnel %q stopped draining at frame %d while %q was stalled",
						id, i, stalled)
					return
				}
			}
		}(id)
		for i := 0; i < frames; i++ {
			if err := routeUntilAccepted(svc, "sess-stall", id, []byte(fmt.Sprintf("%s#%d", id, i))); err != nil {
				t.Fatalf("tunnel %q frame %d refused while %q was stalled: %v", id, i, stalled, err)
			}
		}
		<-done
	}

	// And the stalled tunnel is still alive -- refused, not torn down. ws-buffer-full is a wait
	// (B4/D4), so once its consumer drains it must resume rather than have been closed.
	<-streams[stalled].Send()
	if err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-stall", stalled, []byte("resumed")); err != nil {
		t.Fatalf("stalled tunnel did not resume after draining: %v", err)
	}
	select {
	case <-streams[stalled].Done():
		t.Fatalf("stalled tunnel was torn down; a full queue is backpressure, not a terminal state")
	default:
	}
}

// TestEmbedTunnels_ClosePerTunnel pins that CloseTunnel closes exactly one tunnel: the closed id
// refuses with tunnel-closed (D7's third row), and its siblings keep delivering.
func TestEmbedTunnels_ClosePerTunnel(t *testing.T) {
	svc, streams := newMultiTunnelSession(t, "sess-close", multiTunnelIDs)
	const closed = "audio-out"

	if err := svc.CloseTunnel("sess-close", closed); err != nil {
		t.Fatalf("close tunnel %q: %v", closed, err)
	}

	err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-close", closed, []byte("after-close"))
	cause, ok := EmbedRefusalCauseOf(err)
	if !ok || cause != EmbedRefusedTunnelClosed {
		t.Fatalf("route on closed tunnel = %v (cause %q), want tunnel-closed -- a closed tunnel "+
			"must be distinguishable from one that is merely not attached, or the pump waits "+
			"120s on a channel that will never accept another frame", err, cause)
	}

	select {
	case <-streams[closed].Done():
	default:
		t.Fatalf("closed tunnel's stream was not signalled done")
	}

	for _, id := range multiTunnelIDs {
		if id == closed {
			continue
		}
		if err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-close", id, []byte("alive")); err != nil {
			t.Fatalf("sibling tunnel %q refused after %q closed: %v -- CloseTunnel must close one "+
				"tunnel, not the session's embed surface", id, closed, err)
		}
		select {
		case <-streams[id].Done():
			t.Fatalf("sibling tunnel %q was torn down by closing %q", id, closed)
		default:
		}
	}
}

// TestEmbedTunnels_RegisterRefusesAboveTheCeiling pins the guard at MaxTunnelsPerSession, and that
// the refusal names the limit: "too many tunnels" alone leaves an author guessing what the limit is
// and whether their 9 was one too many or five.
func TestEmbedTunnels_RegisterRefusesAboveTheCeiling(t *testing.T) {
	ids := make([]string, 0, domain.MaxTunnelsPerSession+1)
	for i := 0; i <= domain.MaxTunnelsPerSession; i++ {
		ids = append(ids, fmt.Sprintf("t%d", i))
	}

	svc := newTestEmbedTunnelService()
	_, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: "sess-over",
		PluginID:  "com.test.rdp",
		UIEntry:   "ui/rdp.html",
		TunnelIDs: ids,
	})
	if err == nil {
		t.Fatalf("register with %d tunnels succeeded, want refusal at %d",
			len(ids), domain.MaxTunnelsPerSession)
	}
	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprintf("%d", domain.MaxTunnelsPerSession)) {
		t.Fatalf("refusal %q does not name the limit (%d)", msg, domain.MaxTunnelsPerSession)
	}

	// Exactly at the ceiling still works: the guard binds abuse, not the six-channel protocols the
	// ceiling was raised for.
	if _, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: "sess-at",
		PluginID:  "com.test.rdp",
		UIEntry:   "ui/rdp.html",
		TunnelIDs: ids[:domain.MaxTunnelsPerSession],
	}); err != nil {
		t.Fatalf("register with exactly %d tunnels refused: %v", domain.MaxTunnelsPerSession, err)
	}
}

// TestEmbedBackend_ConcurrentAuthorizeKeepsTunnelIDs is the 3.5 fresh-instance rule expressed where
// this stage can break it: ChannelEmbedBackend stores tunnelID during Authorize, so a resolver
// handing out a shared instance makes every tunnel deliver to whichever id authorized last.
//
// Asserted on the backends' own resolved state rather than on delivery, for the reason B2 learned:
// Wire's pumps close over locals, so a shared backend only corrupts when Authorize and Wire happen
// to interleave and a delivery-level assertion passes almost every run.
func TestEmbedBackend_ConcurrentAuthorizeKeepsTunnelIDs(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"

	backends := make([]*ChannelEmbedBackend, len(multiTunnelIDs))
	var wg sync.WaitGroup
	for i, id := range multiTunnelIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			b := NewChannelEmbedBackend("com.test", true, sink, nil, nil)
			if err := b.Authorize(domainplugin.PurposeEmbedStream, "sess-1", id); err != nil {
				t.Errorf("authorize %q: %v", id, err)
				return
			}
			backends[i] = b
		}(i, id)
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}

	for i, id := range multiTunnelIDs {
		b := backends[i]
		if b == nil {
			t.Fatalf("tunnel %q got no backend", id)
		}
		b.mu.Lock()
		got := b.tunnelID
		b.mu.Unlock()
		if got != id {
			t.Fatalf("backend for %q resolved tunnelID %q -- a shared instance would deliver "+
				"every tunnel to the last id that opened", id, got)
		}
	}
}
