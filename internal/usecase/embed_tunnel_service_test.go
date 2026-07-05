package usecase

import (
	"context"
	"errors"
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
