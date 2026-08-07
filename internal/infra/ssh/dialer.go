package ssh

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

const defaultTimeoutSeconds = 15

// sshClientWrapper wraps a gossh.Client and implements domain.SSHClient.
type sshClientWrapper struct {
	client *gossh.Client
}

// NewSession opens a new SSH channel session.
func (w *sshClientWrapper) NewSession() (*gossh.Session, error) {
	return w.client.NewSession()
}

// Client returns the underlying ssh.Client (for SFTP, etc.).
func (w *sshClientWrapper) Client() *gossh.Client {
	return w.client
}

// Close terminates the SSH connection.
func (w *sshClientWrapper) Close() error {
	return w.client.Close()
}

// KeepAlive sends a keepalive request to detect connection loss.
func (w *sshClientWrapper) KeepAlive() error {
	_, _, err := w.client.SendRequest("keepalive@golang.org", true, nil)
	return err
}

// dialUnderContext gives a blocking SSH dial a deadline it does not have.
//
// x/crypto/ssh exposes no cancellable Dial: once the channel-open request is on
// the wire the server answers on its own schedule, and a bastion that accepts
// TCP but never answers direct-tcpip holds the caller forever. Running the dial
// on its own goroutine and selecting on the context is the only way to give the
// caller back its deadline.
//
// The drain is the part that is easy to leave out and expensive to omit. The
// dial keeps running after we return, so its result must still be collected and
// closed - otherwise every cancelled dial abandons a channel the peer believes
// is open, for the life of the SSH connection, and a flapping target is exactly
// the case that both cancels often and eventually succeeds. The channel is
// buffered so the dial goroutine never blocks even if nothing drains it.
func dialUnderContext(ctx context.Context, name string, dial func() (net.Conn, error)) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	safego.GoNamed(name, func() {
		c, err := dial()
		ch <- result{c, err}
	})
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		safego.GoNamed(name+".drain", func() {
			if r := <-ch; r.conn != nil {
				_ = r.conn.Close()
			}
		})
		return nil, ctx.Err()
	}
}

func (w *sshClientWrapper) OpenDirectTCP(ctx context.Context, addr string) (net.Conn, error) {
	return dialUnderContext(ctx, "ssh.openDirectTCP", func() (net.Conn, error) {
		return w.client.Dial("tcp", addr)
	})
}

// ListenTCP takes a context to match the port, not to use it: x/crypto/ssh
// offers no cancellable Listen, and unlike a dial there is nothing to abandon -
// the tcpip-forward request either binds on the server or fails. Cancellation
// belongs to the returned Listener, which the caller closes.
func (w *sshClientWrapper) ListenTCP(_ context.Context, remoteAddr string) (net.Listener, error) {
	host, portStr, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid remote forward address %q: %w", remoteAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid remote forward port %q: %w", portStr, err)
	}
	return w.client.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}

// Dialer implements domain.SSHClientFactory using golang.org/x/crypto/ssh.
type Dialer struct{}

// NewDialer creates a new SSH Dialer.
func NewDialer() *Dialer {
	return &Dialer{}
}

// Create establishes an SSH connection using the provided config.
// When cfg.Transport is set, the connection uses that pre-established net.Conn
// instead of dialing TCP directly (used for bastion/jump chains).
func (d *Dialer) Create(ctx context.Context, cfg domain.SSHClientConfig) (domain.SSHClient, error) {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds <= 0 {
		timeout = defaultTimeoutSeconds * time.Second
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var authMethods []gossh.AuthMethod
	if len(cfg.Signers) > 0 {
		authMethods = append(authMethods, gossh.PublicKeys(cfg.Signers...))
	}
	if cfg.Password != "" {
		authMethods = append(authMethods, gossh.Password(cfg.Password))
	}
	// Plugin-provided methods follow built-in publickey/password so servers prefer cheaper methods first.
	authMethods = append(authMethods, cfg.ExtraAuthMethods...)

	sshConfig := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: cfg.HostKeyCallback,
		Timeout:         timeout,
	}

	var conn net.Conn
	var err error

	// Outbound dialing is either a pre-established transport (jump chains) or direct TCP.
	// A future plugin-provided transport would attach via cfg.Transport, not a new branch here.
	if cfg.Transport != nil {
		conn = cfg.Transport
	} else {
		dialer := net.Dialer{Timeout: timeout}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("ssh tcp dial %s: %w", addr, err)
		}
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}

	client := gossh.NewClient(sshConn, chans, reqs)
	return &sshClientWrapper{client: client}, nil
}

// ParseKeyWithPassphrase attempts to parse a PEM-encoded private key.
// If the key is encrypted and passphrase is empty, returns ErrPassphraseRequired.
// If passphrase is provided, uses ParsePrivateKeyWithPassphrase.
func ParseKeyWithPassphrase(pemBytes []byte, passphrase string) (gossh.Signer, error) {
	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err == nil {
		return signer, nil
	}

	missingErr, ok := err.(*gossh.PassphraseMissingError)
	_ = missingErr

	if !ok {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	if passphrase == "" {
		return nil, domain.ErrPassphraseRequired
	}

	signer, err = gossh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse private key with passphrase: %w", err)
	}
	return signer, nil
}

// privateKeySignerFactory implements domain.PrivateKeySignerFactory using ParseKeyWithPassphrase.
type privateKeySignerFactory struct{}

// NewPrivateKeySignerFactory returns a PEM private key parser (encrypted keys supported).
func NewPrivateKeySignerFactory() domain.PrivateKeySignerFactory {
	return privateKeySignerFactory{}
}

func (privateKeySignerFactory) ParsePrivateKeyWithPassphrase(pemBytes []byte, passphrase string) (gossh.Signer, error) {
	return ParseKeyWithPassphrase(pemBytes, passphrase)
}

// PassphraseCache stores passphrases for encrypted keys in memory.
// It is safe for concurrent use.
type PassphraseCache struct {
	mu    sync.RWMutex
	cache map[string]string
}

var _ domain.PassphraseCache = (*PassphraseCache)(nil)

// NewPassphraseCache creates a new empty passphrase cache.
func NewPassphraseCache() *PassphraseCache {
	return &PassphraseCache{cache: make(map[string]string)}
}

// Get retrieves a cached passphrase for the given identity ID.
func (c *PassphraseCache) Get(identityID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.cache[identityID]
	return p, ok
}

// Set stores a passphrase for the given identity ID.
func (c *PassphraseCache) Set(identityID, passphrase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[identityID] = passphrase
}

// Clear removes all cached passphrases from memory.
func (c *PassphraseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.cache {
		c.cache[k] = ""
		delete(c.cache, k)
	}
}
