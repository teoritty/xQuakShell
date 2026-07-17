package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

type stubRateLimiter struct {
	allow bool
}

func (s stubRateLimiter) AllowN(int) bool {
	return s.allow
}

type stubRateLimiterFactory struct {
	allow bool
}

func (f stubRateLimiterFactory) New(int, int) domain.RateLimiter {
	return stubRateLimiter{allow: f.allow}
}

func newTestEmbedTunnelService() *EmbedTunnelService {
	return NewEmbedTunnelService(stubRateLimiterFactory{allow: true})
}

func TestEmbedTunnelRegisterRevoke(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()

	desc, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: "sess-1",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if desc.UIUrl == "" || desc.TunnelUrl == "" {
		t.Fatal("expected urls in descriptor")
	}

	reg, err := svc.Lookup(extractEmbedToken(desc.UIUrl))
	if err != nil {
		t.Fatal(err)
	}
	if reg.SessionID != "sess-1" {
		t.Fatalf("expected sess-1, got %q", reg.SessionID)
	}

	if err := svc.RevokeBySession("sess-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Lookup(extractEmbedToken(desc.UIUrl)); err == nil {
		t.Fatal("expected lookup fail after revoke")
	}
}

func TestEmbedTunnelInactiveBackpressure(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()
	_, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: "sess-2",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSessionActive("sess-2", false)
	if err := svc.OpenTunnel("sess-2", "main"); err != nil {
		t.Fatal(err)
	}
	err = svc.RouteTunnelFrameFromPlugin(ctx, "sess-2", "main", []byte("x"))
	if err == nil {
		t.Fatal("expected backpressure when tab inactive")
	}
}

func TestEmbedTunnelRegisterReplacesPriorToken(t *testing.T) {
	svc := newTestEmbedTunnelService()
	ctx := context.Background()
	first, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: "sess-3",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: "sess-3",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.UIUrl == second.UIUrl {
		t.Fatal("expected new token on re-register")
	}
}

func TestEmbedTunnelRateLimited(t *testing.T) {
	svc := NewEmbedTunnelService(stubRateLimiterFactory{allow: false})
	ctx := context.Background()
	_, err := svc.Register(ctx, domain.EmbedRegistration{
		SessionID: "sess-4",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSessionActive("sess-4", true)
	if err := svc.OpenTunnel("sess-4", "main"); err != nil {
		t.Fatal(err)
	}
	err = svc.RouteTunnelFrameFromPlugin(ctx, "sess-4", "main", []byte("x"))
	if !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

// TestEmbedTunnelSubscribeOutboundLastSubscriberWins pins the bookkeeping SubscribeOutbound owes,
// mirroring AttachWebSocket's rule for the browser end of the same tunnel: a re-opened channel
// replaces a stale subscription rather than splitting one input stream across two consumers, and a
// stale unsubscribe func must not tear down its successor.
func TestEmbedTunnelSubscribeOutboundLastSubscriberWins(t *testing.T) {
	svc := newTestEmbedTunnelService()

	_, unsubFirst := svc.SubscribeOutbound("sess-sub", "main")
	_, unsubSecond := svc.SubscribeOutbound("sess-sub", "main")

	unsubFirst()
	svc.mu.RLock()
	current := svc.outboundSubLocked("sess-sub", "main")
	svc.mu.RUnlock()
	if current == nil {
		t.Fatal("the superseded subscription's unsubscribe removed its successor")
	}

	unsubSecond()
	unsubSecond() // idempotent: CloseRemote may race the session-close cascade
	svc.mu.RLock()
	current = svc.outboundSubLocked("sess-sub", "main")
	svc.mu.RUnlock()
	if current != nil {
		t.Fatal("unsubscribe left the subscription registered: input would block on a dead consumer")
	}
}

// newRoutedEmbedService registers an active embed session and records every legacy
// session.tunnelData notification the service emits.
func newRoutedEmbedService(t *testing.T, sessionID string) (*EmbedTunnelService, func() int) {
	t.Helper()
	svc := newTestEmbedTunnelService()
	var mu sync.Mutex
	notified := 0
	svc.SetPluginNotifier(func(_ context.Context, _, _, method string, _ []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if method == "session.tunnelData" {
			notified++
		}
		return nil
	})
	if _, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: sessionID,
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	}); err != nil {
		t.Fatal(err)
	}
	svc.SetSessionActive(sessionID, true)
	return svc, func() int {
		mu.Lock()
		defer mu.Unlock()
		return notified
	}
}

// TestRouteTunnelFrameToPluginPrefersTheChannelSubscriber is D1's first branch: an open
// embed-stream channel REPLACES session.tunnelData for that tunnel, it does not supplement it. A
// subscription added alongside the legacy notification would deliver every keystroke twice -- once
// as a binary frame, once as base64 JSON.
//
// It drives InitialCredit("embed-stream") x 3 frames (7.2): a run of one window's worth proves
// nothing about a consumer that has to keep up.
func TestRouteTunnelFrameToPluginPrefersTheChannelSubscriber(t *testing.T) {
	svc, notifications := newRoutedEmbedService(t, "sess-d1")
	sub, unsubscribe := svc.SubscribeOutbound("sess-d1", "main")
	defer unsubscribe()

	total := domainplugin.InitialCredit(domainplugin.PurposeEmbedStream) * 3
	got := make(chan []byte, total)
	go func() {
		for i := 0; i < total; i++ {
			got <- <-sub
		}
	}()

	for i := 0; i < total; i++ {
		if err := svc.RouteTunnelFrameToPlugin(context.Background(), "sess-d1", "main",
			[]byte(fmt.Sprintf("key-%02d", i))); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	for i := 0; i < total; i++ {
		want := fmt.Sprintf("key-%02d", i)
		select {
		case frame := <-got:
			if string(frame) != want {
				t.Fatalf("frame %d = %q, want %q: input arrived out of order", i, frame, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d of %d input frames reached the subscriber", i, total)
		}
	}
	if n := notifications(); n != 0 {
		t.Fatalf("session.tunnelData fired %d times while a channel subscriber was open: the "+
			"plugin received every input event twice (D1)", n)
	}
}

// TestRouteTunnelFrameToPluginNotifiesWithoutASubscriber is D1's other branch: with no
// embed-stream channel open, input still has to reach the plugin the legacy ADR-008 way.
func TestRouteTunnelFrameToPluginNotifiesWithoutASubscriber(t *testing.T) {
	svc, notifications := newRoutedEmbedService(t, "sess-legacy")

	if err := svc.RouteTunnelFrameToPlugin(context.Background(), "sess-legacy", "main", []byte("k")); err != nil {
		t.Fatal(err)
	}
	if n := notifications(); n != 1 {
		t.Fatalf("session.tunnelData fired %d times, want 1: input to a tunnel with no channel "+
			"subscriber must still reach the plugin", n)
	}
}

// TestRouteTunnelFrameToPluginSwitchesTransportOnSubscribe covers the transition in both
// directions, which is where a predicate evaluated in two places drifts apart: the tunnel starts on
// the legacy notification, moves to the channel when one opens, and moves back when it closes.
// Never both, never neither.
func TestRouteTunnelFrameToPluginSwitchesTransportOnSubscribe(t *testing.T) {
	svc, notifications := newRoutedEmbedService(t, "sess-switch")
	ctx := context.Background()

	if err := svc.RouteTunnelFrameToPlugin(ctx, "sess-switch", "main", []byte("before")); err != nil {
		t.Fatal(err)
	}
	if n := notifications(); n != 1 {
		t.Fatalf("before subscribe: notifications = %d, want 1", n)
	}

	sub, unsubscribe := svc.SubscribeOutbound("sess-switch", "main")
	received := make(chan []byte, 1)
	go func() { received <- <-sub }()
	if err := svc.RouteTunnelFrameToPlugin(ctx, "sess-switch", "main", []byte("during")); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-received:
		if string(frame) != "during" {
			t.Fatalf("subscriber got %q, want %q", frame, "during")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the channel subscriber never received the frame")
	}
	if n := notifications(); n != 1 {
		t.Fatalf("during subscribe: notifications = %d, want 1: the channel frame was also notified", n)
	}

	unsubscribe()
	if err := svc.RouteTunnelFrameToPlugin(ctx, "sess-switch", "main", []byte("after")); err != nil {
		t.Fatal(err)
	}
	if n := notifications(); n != 2 {
		t.Fatalf("after unsubscribe: notifications = %d, want 2: input stopped reaching the plugin "+
			"when its channel closed", n)
	}
}
