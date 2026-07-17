package usecase

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// registerEmbedForRefusal registers one session with one attached-or-not tunnel and returns the
// service plus the minted token.
func registerEmbedForRefusal(t *testing.T, allowRate bool) (*EmbedTunnelService, string) {
	t.Helper()
	svc := NewEmbedTunnelService(stubRateLimiterFactory{allow: allowRate})
	desc, err := svc.Register(context.Background(), domain.EmbedRegistration{
		SessionID: "sess-refuse",
		PluginID:  "com.test.vnc",
		UIEntry:   "ui/index.html",
		TunnelIDs: []string{"main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, extractEmbedToken(desc.UIUrl)
}

func assertRefusal(t *testing.T, err error, want EmbedRefusalCause, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a refusal classified as %q, got nil", want)
	}
	cause, ok := EmbedRefusalCauseOf(err)
	if !ok {
		t.Fatalf("expected a classified refusal (%q), got an unclassified error: %v", want, err)
	}
	if cause != want {
		t.Fatalf("expected cause %q, got %q", want, cause)
	}
	// Wrap, never replace: every pre-existing caller matches the sentinel, including
	// ipc.HostServer's JSON-RPC error-code mapping.
	if !errors.Is(err, sentinel) {
		t.Fatalf("classified refusal %q no longer satisfies errors.Is against %v", cause, sentinel)
	}
}

func TestRouteTunnelFrameFromPluginClassifiesFrameTooLarge(t *testing.T) {
	svc, _ := registerEmbedForRefusal(t, true)
	err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-refuse", "main",
		make([]byte, domain.MaxTunnelFrameSize+1))
	assertRefusal(t, err, EmbedRefusedFrameTooLarge, domainplugin.ErrRateLimited)
}

func TestRouteTunnelFrameFromPluginClassifiesSessionGone(t *testing.T) {
	svc, _ := registerEmbedForRefusal(t, true)
	err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-unknown", "main", []byte("x"))
	assertRefusal(t, err, EmbedRefusedSessionGone, domain.ErrSessionNotFound)
}

func TestRouteTunnelFrameFromPluginClassifiesTabInactive(t *testing.T) {
	svc, _ := registerEmbedForRefusal(t, true)
	svc.SetSessionActive("sess-refuse", false)
	err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-refuse", "main", []byte("x"))
	assertRefusal(t, err, EmbedRefusedTabInactive, domainplugin.ErrTerminalBackpressure)
}

func TestRouteTunnelFrameFromPluginClassifiesRateLimited(t *testing.T) {
	svc, _ := registerEmbedForRefusal(t, false)
	svc.SetSessionActive("sess-refuse", true)
	err := svc.RouteTunnelFrameFromPlugin(context.Background(), "sess-refuse", "main", []byte("x"))
	assertRefusal(t, err, EmbedRefusedRateLimited, domainplugin.ErrRateLimited)
}

func TestRouteTunnelFrameFromPluginClassifiesWSBufferFull(t *testing.T) {
	svc, token := registerEmbedForRefusal(t, true)
	svc.SetSessionActive("sess-refuse", true)
	if err := svc.OpenTunnel("sess-refuse", "main"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AttachWebSocket(token, "main"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Fill the browser send queue without draining it, then prove the overflow frame is refused
	// with the momentary cause rather than the minutes-long one it shares a sentinel with.
	for i := 0; i < 256; i++ {
		if err := svc.RouteTunnelFrameFromPlugin(ctx, "sess-refuse", "main", []byte("x")); err != nil {
			t.Fatalf("frame %d refused while the queue still had room: %v", i, err)
		}
	}
	err := svc.RouteTunnelFrameFromPlugin(ctx, "sess-refuse", "main", []byte("x"))
	assertRefusal(t, err, EmbedRefusedWSBufferFull, domainplugin.ErrTerminalBackpressure)
}
