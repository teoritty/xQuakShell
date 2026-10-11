package agentforward

import (
	"bytes"
	"fmt"
	"sync"
)

// Binding is one verified session-bind@openssh.com message: the remote SSH client proved, with the
// destination's own host key signature over the session identifier, that the session it is
// authenticating belongs to that host.
//
// The proof is what makes the destination trustworthy. A session identifier is the exchange hash
// of one key exchange, so a signature over a login request carrying it is usable in that session
// only; if the destination's host key signed the identifier, that session is with that host.
type Binding struct {
	SessionID          []byte
	HostKey            []byte
	HostKeyFingerprint string
	// Forwarding says the remote client will forward the agent on to the destination too, so
	// later requests on this channel may come from further away than the first hop.
	Forwarding bool
}

// Bindings is the session-bind table of one agent channel. OpenSSH keeps it per agent
// connection, and so do we: each `ssh` run on the server opens its own channel and binds its own
// sessions, and one channel's proof says nothing about another's.
type Bindings struct {
	mu   sync.Mutex
	list []Binding
}

// Add records a binding whose signature the caller has already verified.
//
// Re-binding a session to the same host is accepted as a no-op, which is what OpenSSH does for a
// client that retries. Re-binding it to a different host is refused: the session identifier is
// unique per key exchange, so two hosts claiming one is a forgery by construction.
func (b *Bindings) Add(nb Binding) error {
	if len(nb.SessionID) == 0 || len(nb.SessionID) > MaxSessionIDLen || len(nb.HostKey) == 0 || nb.HostKeyFingerprint == "" {
		return fmt.Errorf("%w: incomplete binding", ErrBindingRefused)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, existing := range b.list {
		if !bytes.Equal(existing.SessionID, nb.SessionID) {
			continue
		}
		if bytes.Equal(existing.HostKey, nb.HostKey) {
			return nil
		}
		return fmt.Errorf("%w: session already bound to another host", ErrBindingRefused)
	}
	if len(b.list) >= MaxBindingsPerChannel {
		return fmt.Errorf("%w: too many bindings", ErrBindingRefused)
	}
	b.list = append(b.list, cloneBinding(nb))
	return nil
}

// Lookup returns the binding for a session identifier.
func (b *Bindings) Lookup(sessionID []byte) (Binding, bool) {
	if b == nil {
		return Binding{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, existing := range b.list {
		if bytes.Equal(existing.SessionID, sessionID) {
			return cloneBinding(existing), true
		}
	}
	return Binding{}, false
}

// Destination resolves where a login request's signature can be used.
//
// verified is true only when the request's session was bound on this channel. A hostbound request
// names a host key of its own; when the session is also bound, the two must agree, because the
// signature commits to the key in the data while the user is shown the key from the binding.
func Destination(req UserAuthRequest, b *Bindings) (binding Binding, verified bool, err error) {
	binding, verified = b.Lookup(req.SessionID)
	if !verified {
		return Binding{}, false, nil
	}
	if req.HostKey != nil && !bytes.Equal(req.HostKey, binding.HostKey) {
		return Binding{}, false, ErrHostKeyMismatch
	}
	return binding, true, nil
}

func cloneBinding(b Binding) Binding {
	b.SessionID = bytes.Clone(b.SessionID)
	b.HostKey = bytes.Clone(b.HostKey)
	return b
}
