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
	// these land in the same bucket once keyed by host. Failures below the budget are answered
	// 401 (bad token); the one that spends the budget must be throttled with 429, not panic.
	var lastCode int
	for i := 0; i < maxFailedLookups; i++ {
		req := httptest.NewRequest(http.MethodGet, "/embed/s/bad-token/ui/index.html", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("status after %d failed lookups = %d, want %d", maxFailedLookups, lastCode, http.StatusTooManyRequests)
	}
}

// The bucket is per remote host, and the broker listens only on 127.0.0.1: every embed client in
// the process shares one bucket, and requests proxied by the Wails asset server (no parseable
// RemoteAddr) share the "" bucket. So a single expired session whose iframe is still mounted can
// fill the budget on its own — 20 failed asset or tunnel requests in a minute is one page load or
// one reconnecting WebSocket.
//
// Gating on the bucket *before* the token was examined turned that into a machine-wide outage:
// every other embed UI got 429 with a perfectly valid token. The budget must only ever be charged
// to a request that actually failed a lookup.
func TestBrokerHandler_validTokenSurvivesAFullFailureBucket(t *testing.T) {
	var lookups int
	h := NewBrokerHandler(stubEmbedTunnelPort{
		lookupFn: func(token string) (domain.EmbedRegistration, error) {
			lookups++
			if token != "good-token" {
				return domain.EmbedRegistration{}, domain.ErrSessionNotFound
			}
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

	// The expired session's iframe hammers the broker until the shared bucket is well past budget.
	for i := 0; i < maxFailedLookups*3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/embed/s/stale-token/ui/index.html", nil))
		if rec.Code == http.StatusOK {
			t.Fatalf("request %d with a stale token succeeded", i)
		}
	}

	before := lookups
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/embed/s/good-token/ui/index.html", nil))

	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("a valid token was throttled by another session's failures: one expired embed locks out every embed UI")
	}
	if lookups != before+1 {
		t.Fatalf("Lookup calls = %d, want %d: the valid token was rejected before it was even examined", lookups-before, 1)
	}
	// The resolver has no UI root, so a token that got past the limiter lands on 404 — proof the
	// request reached the routing that follows a successful lookup.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (valid token, no UI root)", rec.Code, http.StatusNotFound)
	}
}

// The failure map must not accumulate a bucket per host that ever failed once. Buckets whose
// window has fully aged out are evicted; before the fix the eviction branch was unreachable,
// because the only key it could have applied to was the one that had just had a timestamp
// appended to it.
func TestBrokerHandler_agedOutFailureBucketsAreEvicted(t *testing.T) {
	h := NewBrokerHandler(stubEmbedTunnelPort{}, func(string) (string, error) {
		return "", domain.ErrSessionNotFound
	})

	stale := httptest.NewRequest(http.MethodGet, "/embed/s/bad/ui/index.html", nil)
	stale.RemoteAddr = "192.0.2.9:5555"
	h.recordFailedLookup(stale)

	// Age that bucket past the window without waiting a real minute.
	h.failMu.Lock()
	h.failures["192.0.2.9"] = []time.Time{time.Now().Add(-2 * failedLookupWindow)}
	h.failMu.Unlock()

	fresh := httptest.NewRequest(http.MethodGet, "/embed/s/bad/ui/index.html", nil)
	fresh.RemoteAddr = "192.0.2.10:5555"
	h.recordFailedLookup(fresh)

	h.failMu.Lock()
	defer h.failMu.Unlock()
	if _, ok := h.failures["192.0.2.9"]; ok {
		t.Fatalf("aged-out bucket was not evicted: failures = %v", h.failures)
	}
	if len(h.failures) != 1 {
		t.Fatalf("failures = %v, want only the live bucket", h.failures)
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
