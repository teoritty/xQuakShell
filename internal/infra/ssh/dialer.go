package ssh

import (
	"context"
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"

	"ssh-client/internal/domain"
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

	sshConfig := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: cfg.HostKeyCallback,
		Timeout:         timeout,
	}

	var conn net.Conn
	var err error

	if cfg.Transport != nil {
		conn = cfg.Transport
	} else if cfg.Proxy != nil && cfg.Proxy.Host != "" && cfg.Proxy.Port > 0 {
		var auth *proxy.Auth
		if cfg.Proxy.Username != "" || cfg.Proxy.Password != "" {
			auth = &proxy.Auth{User: cfg.Proxy.Username, Password: cfg.Proxy.Password}
		}
		proxyAddr := fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
		socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 proxy %s: %w", proxyAddr, err)
		}
		conn, err = socksDialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("ssh via socks %s: %w", addr, err)
		}
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
	cache map[string]string
}

var _ domain.PassphraseCache = (*PassphraseCache)(nil)

// NewPassphraseCache creates a new empty passphrase cache.
func NewPassphraseCache() *PassphraseCache {
	return &PassphraseCache{cache: make(map[string]string)}
}

// Get retrieves a cached passphrase for the given identity ID.
func (c *PassphraseCache) Get(identityID string) (string, bool) {
	p, ok := c.cache[identityID]
	return p, ok
}

// Set stores a passphrase for the given identity ID.
func (c *PassphraseCache) Set(identityID, passphrase string) {
	c.cache[identityID] = passphrase
}

// Clear removes all cached passphrases from memory.
func (c *PassphraseCache) Clear() {
	for k := range c.cache {
		c.cache[k] = ""
		delete(c.cache, k)
	}
}
