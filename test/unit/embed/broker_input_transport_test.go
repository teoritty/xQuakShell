// Package embed_test drives the embed broker's browser->plugin leg with the real thing on both
// sides: a real WebSocket, the real BrokerHandler, and the real EmbedTunnelService.
//
// It lives in test/unit rather than beside either half because neither half may import the other
// (infra must not import usecase), and because both halves are already green in isolation -- the
// broker's tests stub the tunnel port, the service's tests call it directly. That is the kind-of-
// test gap the readiness report's 7.1 describes: the seam is exactly what nobody instantiates.
package embed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	infraembed "xquakshell/internal/infra/embed"
	"xquakshell/internal/pkg/ratelimit"
	"xquakshell/internal/usecase"

	"github.com/gorilla/websocket"
)

const testSessionID = "sess-broker"

// newBrokerSeam serves the real BrokerHandler over loopback and returns the service behind it plus
// a counter of legacy session.tunnelData notifications.
func newBrokerSeam(t *testing.T) (*usecase.EmbedTunnelService, string, func() int) {
	t.Helper()
	svc := usecase.NewEmbedTunnelService(ratelimit.Factory{})

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

	desc, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: testSessionID,
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/vnc.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatalf("registerEmbed: %v", err)
	}
	svc.SetSessionActive(testSessionID, true)
	if err := svc.OpenTunnel(testSessionID, "main"); err != nil {
		t.Fatalf("open tunnel: %v", err)
	}

	srv := httptest.NewServer(infraembed.NewBrokerHandler(svc, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + desc.TunnelUrl
	return svc, wsURL, func() int {
		mu.Lock()
		defer mu.Unlock()
		return notified
	}
}

func dialTunnel(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

// TestBrokerInputReachesTheChannelSubscriberExactlyOnce is B3's "done when": bytes go in at the
// broker's pumpWSToPlugin entry point -- a real browser WebSocket -- and must arrive on the
// embed-stream channel's data path exactly once and in order, with the legacy session.tunnelData
// notification never firing (D1).
//
// It drives InitialCredit("embed-stream") x 3 frames (7.2). A run of 8 would pass with the
// subscription's blocking handoff replaced by anything at all, including a buffer that later drops.
func TestBrokerInputReachesTheChannelSubscriberExactlyOnce(t *testing.T) {
	svc, wsURL, notifications := newBrokerSeam(t)

	// This subscription is what ChannelEmbedBackend.Wire takes out when a plugin opens an
	// embed-stream channel for this session's tunnel.
	sub, unsubscribe := svc.SubscribeOutbound(testSessionID, "main")
	defer unsubscribe()
	ws := dialTunnel(t, wsURL)

	total := domainplugin.InitialCredit(domainplugin.PurposeEmbedStream) * 3
	go func() {
		for i := 0; i < total; i++ {
			if err := ws.WriteMessage(websocket.BinaryMessage, []byte(fmt.Sprintf("key-%02d", i))); err != nil {
				return
			}
		}
	}()

	for i := 0; i < total; i++ {
		want := fmt.Sprintf("key-%02d", i)
		select {
		case frame := <-sub:
			if string(frame) != want {
				t.Fatalf("frame %d = %q, want %q: browser input arrived out of order", i, frame, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d input frames crossed the broker seam", i, total)
		}
	}

	select {
	case extra := <-sub:
		t.Fatalf("an extra frame %q arrived: input was delivered more than once", extra)
	case <-time.After(50 * time.Millisecond):
	}
	if n := notifications(); n != 0 {
		t.Fatalf("session.tunnelData fired %d times while the embed-stream channel was open: the "+
			"plugin received every keystroke twice (D1)", n)
	}
}

// TestBrokerInputFallsBackToTheLegacyNotification is the other branch through the same real
// WebSocket: with no channel subscribed, browser input must still reach the plugin as
// session.tunnelData.
func TestBrokerInputFallsBackToTheLegacyNotification(t *testing.T) {
	_, wsURL, notifications := newBrokerSeam(t)
	ws := dialTunnel(t, wsURL)

	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("key")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		if notifications() == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("notifications = %d, want 1: input to a tunnel with no channel subscriber "+
				"never reached the plugin", notifications())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
