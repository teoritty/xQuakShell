// Package agentforward holds the rules that decide what a forwarded SSH agent may do: which data it
// signs, which algorithms it signs with, where a signature may be used and for how long an approval
// lasts. It is pure logic over byte strings and clocks so every rule can be tested — and
// mutation-tested — without a network or a key. ADR-022 is the record of why each rule exists.
package agentforward

import "errors"

var (
	// ErrSignDataRejected means the data a remote asked us to sign is not a publickey login
	// request for the key it named. The agent signs logins only (ADR-022 §2).
	ErrSignDataRejected = errors.New("agent forwarding: data is not a login request")
	// ErrAlgorithmRefused means the key, the algorithm in the request and the signature flags
	// disagree, or the algorithm is one this application does not sign with (SHA-1 RSA).
	ErrAlgorithmRefused = errors.New("agent forwarding: signature algorithm refused")
	// ErrBindingRefused means a session-bind could not be recorded: malformed, a conflicting
	// rebind of a session already bound to another host, or too many bindings on one channel.
	ErrBindingRefused = errors.New("agent forwarding: session binding refused")
	// ErrHostKeyMismatch means a hostbound login names a server host key other than the one the
	// session was bound to — the request is for a different destination than the binding claims.
	ErrHostKeyMismatch = errors.New("agent forwarding: login names a different host than its binding")
	// ErrRateLimited means a session asked for more signatures than the limiter allows.
	ErrRateLimited = errors.New("agent forwarding: too many signature requests")
	// ErrInvalidConfig means a connection's forwarding settings are malformed.
	ErrInvalidConfig = errors.New("agent forwarding: invalid configuration")
)
