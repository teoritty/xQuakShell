package ssh

import (
	"fmt"
	"net"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// HostKeyChecker wraps a KnownHostsRepository to provide an ssh.HostKeyCallback.
type HostKeyChecker struct {
	repo domain.KnownHostsRepository
}

// NewHostKeyChecker creates a HostKeyChecker backed by the given repository.
func NewHostKeyChecker(repo domain.KnownHostsRepository) *HostKeyChecker {
	return &HostKeyChecker{repo: repo}
}

// HostKeyError wraps a host key verification error with the offending public key,
// so callers can extract it for UI display without losing it.
type HostKeyError struct {
	Err  error
	Host string
	Key  gossh.PublicKey
}

func (e *HostKeyError) Error() string { return e.Err.Error() }
func (e *HostKeyError) Unwrap() error { return e.Err }

// HostKeyCallback returns a function compatible with ssh.ClientConfig.HostKeyCallback.
// It does NOT automatically add unknown keys — the caller must handle ErrUnknownHost.
// Errors are wrapped in HostKeyError so the public key is available to the caller.
func (c *HostKeyChecker) HostKeyCallback() gossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		host := normalizeHostPort(hostname, remote)

		err := c.repo.Check(host, key)
		if err != nil {
			return &HostKeyError{Err: err, Host: host, Key: key}
		}
		return nil
	}
}

// HostKeyInfo holds details about a remote host key for UX display.
type HostKeyInfo struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	KeyBase64   string `json:"keyBase64"`
}

// ExtractHostKeyInfo creates a HostKeyInfo struct from a host key callback error context.
// Use this when presenting unknown/mismatched host keys in the UI.
func ExtractHostKeyInfo(host string, key gossh.PublicKey) HostKeyInfo {
	return HostKeyInfo{
		Host:        host,
		KeyType:     key.Type(),
		Fingerprint: gossh.FingerprintSHA256(key),
		KeyBase64:   string(gossh.MarshalAuthorizedKey(key)),
	}
}

// normalizeHostPort extracts the host:port string from SSH callback parameters.
func normalizeHostPort(hostname string, remote net.Addr) string {
	if hostname != "" {
		host, port, err := net.SplitHostPort(hostname)
		if err == nil {
			if port == "22" {
				return host
			}
			return fmt.Sprintf("[%s]:%s", host, port)
		}
		return hostname
	}
	if remote != nil {
		return remote.String()
	}
	return ""
}
