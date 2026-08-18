// Package httpsafe carries the HTTP transport rules every forge client has to apply identically.
package httpsafe

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrInsecureRedirect indicates a redirect tried to move the request off TLS, or onto a scheme
// the forge clients do not speak.
var ErrInsecureRedirect = errors.New("refusing redirect that drops TLS")

// RefuseTransportDowngrade stops a redirect chain from silently leaving TLS.
//
// Release asset downloads follow a URL that arrives inside an API response rather than being built
// by the caller, and the default http.Client follows wherever it points - including from https to
// plain http, across up to ten hops, with nothing reported. The asset itself is checksum-verified,
// but SHA256SUMS is fetched by the one path that cannot be (it is the listing), so the transport is
// the only thing protecting the file everything else is checked against.
//
// A request that STARTS on http is left alone: that is httptest in the unit tests and a plain-HTTP
// self-hosted base someone configured deliberately. What is refused is a chain that began on https
// and does not stay there - a downgrade nobody asked for - and any hop onto a scheme that is neither.
//
// It lives here rather than in one forge's package because GitHub and GitLab both redirect asset
// downloads to a CDN, so both need it and a copy that drifts is a hole in whichever copy lost.
func RefuseTransportDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("%w: stopped after %d redirects", ErrInsecureRedirect, len(via))
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: redirect to unsupported scheme %q", ErrInsecureRedirect, req.URL.Scheme)
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: %s redirected to %s", ErrInsecureRedirect, via[0].URL.Host, req.URL.Scheme)
	}
	return nil
}
