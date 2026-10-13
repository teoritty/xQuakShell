// Package syncclient is the HTTPS client for xqsyncd (ADR-023). It pins the server's identity key,
// not a certificate: hello returns the identity key's signature over the TLS key, and from then on
// every connection must present exactly that TLS key, or the request is not sent at all.
package syncclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

// Config names the server and, once the user has confirmed it, the identity key to hold it to.
type Config struct {
	// BaseURL is the server's https:// address.
	BaseURL string
	// PinnedIdentity is the server's Ed25519 identity key the user confirmed. Empty means first use:
	// Hello then reports the key for the user to compare with the fingerprint the server admin
	// printed; once they agree, every later client is built with the key pinned here.
	PinnedIdentity []byte
	// Timeout bounds each request; zero means 30 seconds.
	Timeout time.Duration
}

// Client talks to one sync server. It is safe for concurrent use.
type Client struct {
	base    *url.URL
	timeout time.Duration
	hello   *http.Client
	api     *http.Client

	mu      sync.Mutex
	pinned  []byte
	trusted []byte // SPKI verified by the last hello
	access  string
}

// New returns a client for cfg.BaseURL, which must be https.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("syncclient: server address must be an https:// URL")
	}
	if len(cfg.PinnedIdentity) != 0 && len(cfg.PinnedIdentity) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("syncclient: pinned identity key must be %d bytes", ed25519.PublicKeySize)
	}
	c := &Client{base: u, timeout: cfg.Timeout, pinned: slices.Clone(cfg.PinnedIdentity)}
	if c.timeout == 0 {
		c.timeout = 30 * time.Second
	}
	c.hello = c.httpClient(c.anyPeer, true)
	c.api = c.httpClient(c.verifiedPeer, false)
	return c, nil
}

// httpClient builds a client whose TLS trust is the given connection check instead of a CA chain.
//
// InsecureSkipVerify disables only the CA chain, which a self-hosted server with a self-signed
// certificate cannot pass. VerifyConnection still runs on every handshake and replaces it with a
// stricter check: the presented key must be the one the pinned identity key signed. The hello
// client accepts any key, but sends nothing secret, and Hello verifies the signature over the key
// of the very connection it arrived on before anything else is sent anywhere.
func (c *Client) httpClient(verify func(tls.ConnectionState) error, oneShot bool) *http.Client {
	tr := &http.Transport{
		TLSClientConfig:     c.tlsConfig(verify),
		DisableKeepAlives:   oneShot,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: c.timeout, CheckRedirect: refuseRedirects}
}

func (c *Client) tlsConfig(verify func(tls.ConnectionState) error) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // #nosec G402 -- replaced by VerifyConnection, see httpClient
		VerifyConnection:   verify,
	}
}

// refuseRedirects keeps every request on the verified server: a redirect would carry the bearer
// token to whatever address a hostile reply names.
func refuseRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func (c *Client) anyPeer(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return synctrust.ErrServerIdentity
	}
	return nil
}

func (c *Client) verifiedPeer(cs tls.ConnectionState) error {
	c.mu.Lock()
	trusted := c.trusted
	c.mu.Unlock()
	if trusted == nil || len(cs.PeerCertificates) == 0 || !bytes.Equal(cs.PeerCertificates[0].RawSubjectPublicKeyInfo, trusted) {
		return synctrust.ErrServerIdentity
	}
	return nil
}

// SetAccessToken sets the bearer token sent with authenticated requests; empty clears it.
func (c *Client) SetAccessToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.access = token
}

// CloseIdleConnections drops pooled connections, so the next request handshakes afresh.
func (c *Client) CloseIdleConnections() {
	c.hello.CloseIdleConnections()
	c.api.CloseIdleConnections()
}

// Close releases the client's connections.
func (c *Client) Close() { c.CloseIdleConnections() }

// Hello opens a session: it negotiates the protocol and suite and verifies that the TLS key of the
// connection it ran on is signed by the server's identity key - the pinned one, if any. On success
// that TLS key becomes the only one later requests will talk to.
func (c *Client) Hello(ctx context.Context) (synctrust.HelloInfo, error) {
	req := syncwire.Hello{ClientProtocol: wireVersion(vaultsync.CurrentProtocol), Suite: uint16(vaultsync.CurrentSuite)}
	var reply syncwire.HelloReply
	spki, err := c.do(ctx, call{client: c.hello, method: http.MethodPost, path: "/v1/hello", body: req, reply: &reply})
	if err != nil {
		return synctrust.HelloInfo{}, err
	}
	if err := c.verifyIdentity(reply, spki); err != nil {
		return synctrust.HelloInfo{}, err
	}
	res, err := negotiate(reply)
	if err != nil {
		return synctrust.HelloInfo{}, err
	}
	c.mu.Lock()
	c.trusted = spki
	c.mu.Unlock()
	return res, nil
}

func (c *Client) verifyIdentity(reply syncwire.HelloReply, spki []byte) error {
	c.mu.Lock()
	pinned := c.pinned
	c.mu.Unlock()
	if len(reply.ServerIdentityKey) != ed25519.PublicKeySize {
		return synctrust.ErrServerIdentity
	}
	if pinned != nil && !bytes.Equal(pinned, reply.ServerIdentityKey) {
		return synctrust.ErrServerIdentity
	}
	msg, err := vaultsync.ServerTLSMessage(spki)
	if err != nil {
		return fmt.Errorf("%w: %w", synctrust.ErrServerIdentity, err)
	}
	if !ed25519.Verify(reply.ServerIdentityKey, msg, reply.TLSKeySig) {
		return synctrust.ErrServerIdentity
	}
	return nil
}

// negotiate checks the server's offer and binds the password exchange to it: the context covers the
// identity key, protocol range and suites, so a downgraded offer makes the OPAQUE exchange fail
// instead of succeed against a weaker one.
func negotiate(reply syncwire.HelloReply) (synctrust.HelloInfo, error) {
	if _, err := vaultsync.NegotiateProtocol(vaultsync.CurrentProtocol, domainVersion(reply.ServerMin), domainVersion(reply.ServerMax)); err != nil {
		return synctrust.HelloInfo{}, fmt.Errorf("%w: %w", synctrust.ErrRemoteUnsupported, err)
	}
	suites := make([]vaultsync.SuiteID, 0, len(reply.Suites))
	for _, s := range reply.Suites {
		suites = append(suites, vaultsync.SuiteID(s))
	}
	if !slices.Contains(suites, vaultsync.CurrentSuite) {
		return synctrust.HelloInfo{}, synctrust.ErrRemoteUnsupported
	}
	hc, err := vaultsync.HelloContext(reply.ServerIdentityKey, domainVersion(reply.ServerMin), domainVersion(reply.ServerMax), suites)
	if err != nil {
		return synctrust.HelloInfo{}, errors.Join(synctrust.ErrRemoteProtocol, err)
	}
	return synctrust.HelloInfo{IdentityKey: slices.Clone(reply.ServerIdentityKey), Context: hc}, nil
}

func wireVersion(v vaultsync.ProtocolVersion) syncwire.Version {
	return syncwire.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
}

func domainVersion(v syncwire.Version) vaultsync.ProtocolVersion {
	return vaultsync.ProtocolVersion{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
}
