package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func redirectRequest(t *testing.T, from, to string) (*http.Request, []*http.Request) {
	t.Helper()
	origin, err := url.Parse(from)
	if err != nil {
		t.Fatalf("parse %q: %v", from, err)
	}
	target, err := url.Parse(to)
	if err != nil {
		t.Fatalf("parse %q: %v", to, err)
	}
	return &http.Request{URL: target}, []*http.Request{{URL: origin}}
}

// BrowserDownloadURL arrives inside an API response rather than being built here, and the default
// client follows wherever it points - including off TLS, silently. The asset itself is now
// checksum-verified, but SHA256SUMS is fetched by the one path that cannot be, so the transport is
// the only thing protecting the file everything else is checked against.
func TestRefuseTransportDowngradeStopsHTTPSToHTTP(t *testing.T) {
	req, via := redirectRequest(t, "https://api.github.com/assets/1", "http://evil.example.com/asset")

	err := refuseTransportDowngrade(req, via)

	if !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("refuseTransportDowngrade = %v, want ErrInsecureRedirect", err)
	}
}

// A chain that stays on TLS is ordinary: GitHub redirects asset downloads to its CDN on every
// request, so refusing this would break every install.
func TestRefuseTransportDowngradeAllowsHTTPSToHTTPS(t *testing.T) {
	req, via := redirectRequest(t, "https://api.github.com/assets/1", "https://objects.githubusercontent.com/asset")

	if err := refuseTransportDowngrade(req, via); err != nil {
		t.Fatalf("refuseTransportDowngrade = %v, want nil for an https-to-https hop", err)
	}
}

// A request that starts on http is httptest in the unit tests, or a plain-HTTP Enterprise base
// someone configured deliberately. Neither is a downgrade, because there was nothing to drop.
func TestRefuseTransportDowngradeLeavesAPlainStartAlone(t *testing.T) {
	req, via := redirectRequest(t, "http://127.0.0.1:1234/assets/1", "http://127.0.0.1:1234/asset")

	if err := refuseTransportDowngrade(req, via); err != nil {
		t.Fatalf("refuseTransportDowngrade = %v, want nil when the chain never had TLS", err)
	}
}

func TestRefuseTransportDowngradeRejectsAnUnsupportedScheme(t *testing.T) {
	for _, scheme := range []string{"file:///etc/passwd", "ftp://example.com/asset", "gopher://example.com"} {
		t.Run(scheme, func(t *testing.T) {
			req, via := redirectRequest(t, "https://api.github.com/assets/1", scheme)
			if err := refuseTransportDowngrade(req, via); !errors.Is(err, ErrInsecureRedirect) {
				t.Fatalf("refuseTransportDowngrade(%q) = %v, want ErrInsecureRedirect", scheme, err)
			}
		})
	}
}

// A chain that started on plain http is exempt from the downgrade rule, so the scheme check is the
// only thing standing between it and a redirect to file:// or ftp://. Without this case the
// downgrade branch masks the scheme branch entirely and deleting the latter changes nothing that
// any test observes.
func TestRefuseTransportDowngradeRejectsAnUnsupportedSchemeFromAPlainStart(t *testing.T) {
	for _, scheme := range []string{"file:///etc/passwd", "ftp://example.com/asset"} {
		t.Run(scheme, func(t *testing.T) {
			req, via := redirectRequest(t, "http://127.0.0.1:1234/assets/1", scheme)
			if err := refuseTransportDowngrade(req, via); !errors.Is(err, ErrInsecureRedirect) {
				t.Fatalf("refuseTransportDowngrade(%q) from an http start = %v, want ErrInsecureRedirect", scheme, err)
			}
		})
	}
}

func TestRefuseTransportDowngradeStopsAnEndlessChain(t *testing.T) {
	req, _ := redirectRequest(t, "https://api.github.com/a", "https://api.github.com/b")
	var via []*http.Request
	for range 10 {
		v, _ := redirectRequest(t, "https://api.github.com/a", "https://api.github.com/b")
		via = append(via, v)
	}

	if err := refuseTransportDowngrade(req, via); !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("refuseTransportDowngrade after 10 hops = %v, want ErrInsecureRedirect", err)
	}
}

// The policy has to be attached to the client, not merely defined. A downgrade guard nobody
// installed is the situation this replaced.
func TestClientInstallsTheRedirectPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL)
	if c.httpClient.CheckRedirect == nil {
		t.Fatal("the client follows redirects with no policy at all")
	}

	// And an ordinary plain-HTTP fetch still works, which is what the unit tests depend on.
	body, err := c.DownloadAsset(context.Background(), srv.URL+"/asset")
	if err != nil {
		t.Fatalf("DownloadAsset over httptest = %v, want nil", err)
	}
	_ = body.Close()
}
