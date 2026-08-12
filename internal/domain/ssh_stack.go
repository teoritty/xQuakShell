package domain

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// ParseAuthorizedSSHKey parses an OpenSSH authorized_keys line or bare key blob.
func ParseAuthorizedSSHKey(authorizedKey string) (ssh.PublicKey, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return nil, fmt.Errorf("parse authorized key: %w", err)
	}
	return key, nil
}

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
//
// SetWithTTL exists because a cache that only holds entries until the vault locks cannot express
// "forget this in fifteen minutes", and an unlocked application left unattended should stop being
// able to authenticate on its own. Forget is the single-key counterpart of Clear, needed when a
// key's passphrase changes and the old one must not survive to be tried against the new blob.
type PassphraseCache interface {
	Get(identityID string) (passphrase string, ok bool)
	Set(identityID, passphrase string)
	SetWithTTL(identityID, passphrase string, ttl time.Duration)
	Forget(identityID string)
	Clear()
}

// HostKeyCallbackBuilder produces ssh.HostKeyCallback implementations bound to known_hosts.
type HostKeyCallbackBuilder interface {
	Build(repo KnownHostsRepository) ssh.HostKeyCallback
}

// PluginAuthMethodBuilder constructs a domain.AuthMethod backed by a
// PluginAuthProvider, without usecase needing to import golang.org/x/crypto/ssh.
type PluginAuthMethodBuilder interface {
	BuildKeyboardInteractive(ctx context.Context, provider PluginAuthProvider, attemptID string, method PluginAuthMethod) AuthMethod
	BuildPublicKey(ctx context.Context, provider PluginAuthProvider, attemptID string, method PluginAuthMethod) (AuthMethod, error)
}

// JumpHopAuthResolver resolves SSH signers, password, and plugin auth for a single jump hop.
// release, when non-nil, must be called after the hop SSH handshake completes.
type JumpHopAuthResolver func(hop JumpHop) ([]ssh.Signer, string, []AuthMethod, func(), error)

// JumpTransportBuilder builds a net.Conn to the target over a jump hop chain (bastion TCP forwarding).
type JumpTransportBuilder interface {
	BuildChain(
		ctx context.Context,
		hops []JumpHop,
		targetHost string,
		targetPort int,
		timeoutSeconds int,
		factory SSHClientFactory,
		hostKeyCallback ssh.HostKeyCallback,
		resolveHopAuth JumpHopAuthResolver,
	) (transport net.Conn, cleanup func(), err error)
}
