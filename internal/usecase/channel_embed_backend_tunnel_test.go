package usecase

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// embedTunnelFixture registers one embed session against the REAL EmbedTunnelService and wires a
// real ChannelEmbedBackend to it. These tests deliberately use no sink double: the D7 defect is a
// property of EmbedTunnelService's own seam (wsConns[tunnelID] absent meaning two opposite things),
// so a fake sink would only ever test the fake.
func embedTunnelFixture(t *testing.T, tunnelID string) (*EmbedTunnelService, *fakeEmbedDataPath, chan closeReason, domain.SessionEmbedDescriptor) {
	t.Helper()
	svc := NewEmbedTunnelService(stubRateLimiterFactory{allow: true})
	desc, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: "sess-1",
		PluginID:  "com.test",
		UIEntry:   "ui/index.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}

	svc.SetSessionActive("sess-1", true)

	closes := make(chan closeReason, 4)
	data := newFakeEmbedDataPath(0)
	backend := NewChannelEmbedBackend("com.test", true, realEmbedSink{svc}, nil, func(_ uint32, reason, message string) {
		closes <- closeReason{reason, message}
	})
	backend.ackCeiling = 5 * time.Second
	backend.retryInterval = 2 * time.Millisecond
	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", tunnelID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	handle := mustChannelHandle(t, 7, "com.test", domainplugin.PurposeEmbedStream, "sess-1", "", data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	t.Cleanup(func() { backend.CloseRemote() })
	return svc, data, closes, desc
}

type closeReason struct{ reason, message string }

// realEmbedSink is the real EmbedTunnelService with ONLY ownership stubbed: PluginIDForSession
// reads the SessionRegistry, which is SessionManager's business and not this seam's. Both frame
// routing directions — the code under test — are the real service's.
type realEmbedSink struct{ *EmbedTunnelService }

func (realEmbedSink) PluginIDForSession(string) (string, bool) { return "com.test", true }

// drainBrowser stands in for the iframe's WebSocket consumer: it counts frames genuinely taken.
func drainBrowser(stream domain.EmbedTunnelStream, delivered *atomic.Int64) {
	go func() {
		for {
			select {
			case <-stream.Send():
				delivered.Add(1)
			case <-stream.Done():
				return
			}
		}
	}()
}

// feedInbound pushes n frames at the pump's Recv side, driving past InitialCredit x 3 (7.2).
func feedInbound(data *fakeEmbedDataPath, n int, payload string) {
	go func() {
		for i := 0; i < n; i++ {
			select {
			case data.inbound <- []byte(payload):
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()
}

func waitForDelivered(t *testing.T, delivered *atomic.Int64, want int, what string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for delivered.Load() < int64(want) {
		select {
		case <-deadline:
			t.Fatalf("only %d of %d frames %s", delivered.Load(), want, what)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestChannelEmbedBackend_TunnelClosedUnderChannelStopsAckingFrames is B4's test 4.
//
// Its load-bearing assertion is the ACK COUNT, not the closure. A test that only checks "the
// channel closed" PASSES WHILE THE BUG IS PRESENT: with 3.12's `conn == nil -> return nil`, every
// post-close frame looks delivered, the pump Acks it, the plugin's window reopens and the
// framebuffer delta is destroyed with no error on any path. Frames Acked after the tunnel is gone
// are precisely that silent success.
func TestChannelEmbedBackend_TunnelClosedUnderChannelStopsAckingFrames(t *testing.T) {
	svc, data, closes, desc := embedTunnelFixture(t, "main")

	stream, _, err := svc.AttachWebSocket(extractEmbedToken(desc.UIUrl), "main")
	if err != nil {
		t.Fatal(err)
	}
	var delivered atomic.Int64
	drainBrowser(stream, &delivered)

	feedInbound(data, embedTestFrames, "f")
	waitForDelivered(t, &delivered, embedTestFrames, "reached the browser before the close")

	// The tunnel goes away under the still-open channel.
	if err := svc.CloseTunnel("sess-1", "main"); err != nil {
		t.Fatal(err)
	}
	acksAtClose := data.ackCount()

	// More frames arrive, as a plugin that has not yet learned its tunnel died will send.
	feedInbound(data, embedTestFrames, "post-close")

	select {
	case got := <-closes:
		if got.reason != string(EmbedRefusedTunnelClosed) {
			t.Fatalf("close reason = %q, want %q", got.reason, EmbedRefusedTunnelClosed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the tunnel closed under the channel but the channel was never closed with a reason")
	}

	if acks := data.ackCount(); acks != acksAtClose {
		t.Fatalf("acks = %d, want %d — %d frame(s) were Acked after the tunnel closed: the host reported delivery for frames that reached nobody (the 3.12 silent drop)",
			acks, acksAtClose, acks-acksAtClose)
	}
}

// TestChannelEmbedBackend_NotYetAttachedTunnelWaitsAndResumes is D7's transient row: a plugin may
// channel.open and push an RFB handshake before the iframe finishes loading and calls
// AttachWebSocket. That is a WAIT — no teardown, no frame loss — and every held frame must flow
// once the browser attaches.
func TestChannelEmbedBackend_NotYetAttachedTunnelWaitsAndResumes(t *testing.T) {
	svc, data, closes, desc := embedTunnelFixture(t, "main")
	if err := svc.OpenTunnel("sess-1", "main"); err != nil {
		t.Fatal(err)
	}

	feedInbound(data, embedTestFrames, "f")

	// The iframe is still loading.
	select {
	case got := <-closes:
		t.Fatalf("channel closed with %q while the browser had merely not attached yet", got.reason)
	case <-time.After(150 * time.Millisecond):
	}
	if acks := data.ackCount(); acks != 0 {
		t.Fatalf("acks = %d, want 0 — nothing has been delivered to anyone yet", acks)
	}

	// The iframe loads and attaches.
	stream, _, err := svc.AttachWebSocket(extractEmbedToken(desc.UIUrl), "main")
	if err != nil {
		t.Fatal(err)
	}
	var delivered atomic.Int64
	drainBrowser(stream, &delivered)

	waitForDelivered(t, &delivered, embedTestFrames, "reached the browser after it attached — frames held during the wait were lost")
	select {
	case got := <-closes:
		t.Fatalf("channel closed with %q after the browser attached", got.reason)
	default:
	}
}

// TestChannelEmbedBackend_UnknownTunnelClosesAsPluginBug is D7's third row: an id this session
// never registered is not a race — the plugin is addressing something that never existed. Terminal,
// and the message must say it is the plugin's bug rather than blame the browser.
func TestChannelEmbedBackend_UnknownTunnelClosesAsPluginBug(t *testing.T) {
	_, data, closes, _ := embedTunnelFixture(t, "never-registered")

	feedInbound(data, 1, "f")

	select {
	case got := <-closes:
		if got.reason != string(EmbedRefusedTunnelUnknown) {
			t.Fatalf("close reason = %q, want %q", got.reason, EmbedRefusedTunnelUnknown)
		}
		if !strings.Contains(got.message, "plugin") {
			t.Fatalf("close message = %q, want it to name this as a plugin bug", got.message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a frame for a never-registered tunnel was silently accepted instead of closing the channel")
	}
	if acks := data.ackCount(); acks != 0 {
		t.Fatalf("acks = %d, want 0 — the frame reached nobody", acks)
	}
}
