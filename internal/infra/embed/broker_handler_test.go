package embed

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

type stubEmbedTunnelPort struct {
	lookupFn func(token string) (domain.EmbedRegistration, error)
}

func (s stubEmbedTunnelPort) Lookup(token string) (domain.EmbedRegistration, error) {
	if s.lookupFn != nil {
		return s.lookupFn(token)
	}
	return domain.EmbedRegistration{}, domain.ErrSessionNotFound
}

func (s stubEmbedTunnelPort) AttachWebSocket(token, tunnelID string) (domain.EmbedTunnelStream, domain.EmbedRegistration, error) {
	return nil, domain.EmbedRegistration{}, domain.ErrSessionNotFound
}

func (s stubEmbedTunnelPort) RouteTunnelFrameToPlugin(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

var _ domain.EmbedTunnelPort = stubEmbedTunnelPort{}

func TestBrokerHandler_nilTunnelsNotFound(t *testing.T) {
	h := &BrokerHandler{}
	req := httptest.NewRequest(http.MethodGet, "/embed/s/token/ui/index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestBrokerHandler_invalidTokenUnauthorized(t *testing.T) {
	h := NewBrokerHandler(stubEmbedTunnelPort{}, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	})
	req := httptest.NewRequest(http.MethodGet, "/embed/s/bad-token/ui/index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestBrokerHandler_repeatedFailedLookupsThrottle(t *testing.T) {
	h := NewBrokerHandler(stubEmbedTunnelPort{}, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	})
	// httptest.NewRequest defaults RemoteAddr to "192.0.2.1:1234" for every request, so all of
	// these land in the same bucket once keyed by host. maxFailedLookups failures should be
	// tolerated with 401 (bad token); the next one must be throttled with 429, not panic.
	var lastCode int
	for i := 0; i < maxFailedLookups+1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/embed/s/bad-token/ui/index.html", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("status after %d failed lookups = %d, want %d", maxFailedLookups+1, lastCode, http.StatusTooManyRequests)
	}
}

func TestBrokerHandler_doesNotLogRawToken(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	const token = "super-secret-embed-token-value"
	h := NewBrokerHandler(stubEmbedTunnelPort{
		lookupFn: func(token string) (domain.EmbedRegistration, error) {
			return domain.EmbedRegistration{}, domain.ErrSessionNotFound
		},
	}, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	})
	req := httptest.NewRequest(http.MethodGet, "/embed/s/"+token+"/ui/index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("log output contains raw token: %q", out)
	}
	tag := tokenTag(token)
	if !strings.Contains(out, tag) {
		t.Fatalf("log output missing stable token tag %q: %q", tag, out)
	}
}

func TestBrokerHandler_validTokenUIForbiddenWithoutRoot(t *testing.T) {
	h := NewBrokerHandler(stubEmbedTunnelPort{
		lookupFn: func(token string) (domain.EmbedRegistration, error) {
			return domain.EmbedRegistration{
				Token:     token,
				SessionID: "sess-1",
				PluginID:  "plugin-1",
				UIEntry:   "index.html",
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	})
	req := httptest.NewRequest(http.MethodGet, "/embed/s/good-token/ui/index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
