package domain

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

// HostKeyVerificationError wraps ErrUnknownHost or ErrHostKeyMismatch together with the
// key info so callers can show a host key prompt without depending on the ssh library.
type HostKeyVerificationError struct {
	Err  error
	Host string
	Info HostKeyInfo // filled by infra when the error is created
}

func (e *HostKeyVerificationError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *HostKeyVerificationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PassphraseCache stores passphrases for encrypted keys in memory during the session lifetime.
type PassphraseCache interface {
	Get(identityID string) (passphrase string, ok bool)
	Set(identityID, passphrase string)
	Clear()
}

// HostKeyCallbackBuilder produces ssh.HostKeyCallback implementations bound to known_hosts.
type HostKeyCallbackBuilder interface {
	Build(repo KnownHostsRepository) ssh.HostKeyCallback
}

// JumpHopAuthResolver resolves SSH signers and password for a single jump hop.
type JumpHopAuthResolver func(hop JumpHop) ([]ssh.Signer, string, error)

// JumpTransportBuilder builds a net.Conn to the target over a jump hop chain (bastion TCP forwarding).
type JumpTransportBuilder interface {
	BuildChain(
		ctx context.Context,
		hops []JumpHop,
		targetHost string,
		targetPort int,
		timeoutSeconds int,
		proxyAuth *ProxyAuth,
		factory SSHClientFactory,
		hostKeyCallback ssh.HostKeyCallback,
		resolveHopAuth JumpHopAuthResolver,
	) (transport net.Conn, cleanup func(), err error)
}

// PrivateKeySignerFactory parses PEM private keys, optionally with passphrase.
type PrivateKeySignerFactory interface {
	ParsePrivateKeyWithPassphrase(pemBytes []byte, passphrase string) (ssh.Signer, error)
}
