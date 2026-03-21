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

// HostKeyCallback returns a function compatible with ssh.ClientConfig.HostKeyCallback.
// It does NOT automatically add unknown keys — the caller must handle ErrUnknownHost.
// Errors are wrapped in HostKeyVerificationError so the public key is available to the caller.
func (c *HostKeyChecker) HostKeyCallback() gossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		host := normalizeHostPort(hostname, remote)

		err := c.repo.Check(host, key)
		if err != nil {
			return &domain.HostKeyVerificationError{Err: err, Host: host, Key: key}
		}
		return nil
	}
}

// hostKeyCallbackBuilder implements domain.HostKeyCallbackBuilder.
type hostKeyCallbackBuilder struct{}

// NewHostKeyCallbackBuilder returns a builder that produces host key callbacks from known_hosts.
func NewHostKeyCallbackBuilder() domain.HostKeyCallbackBuilder {
	return hostKeyCallbackBuilder{}
}

func (hostKeyCallbackBuilder) Build(repo domain.KnownHostsRepository) gossh.HostKeyCallback {
	return NewHostKeyChecker(repo).HostKeyCallback()
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
