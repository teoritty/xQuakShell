package agentfwd

import (
	"fmt"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain/agentforward"
)

const sessionBindExtension = "session-bind@openssh.com"

type sessionBindMsg struct {
	HostKey    []byte
	SessionID  []byte
	Signature  []byte
	Forwarding bool
}

// verifySessionBind checks that the destination's host key signed the session identifier.
//
// That signature is the whole value of the message. Without it, a session-bind is a claim made by
// whoever holds the agent socket, and the dialog would show the user the name of a host the
// attacker picked. With it, the identifier — unique to one key exchange — is proven to belong to a
// session with that host, so a signature over a login carrying it is useless anywhere else.
func verifySessionBind(contents []byte) (agentforward.Binding, error) {
	var msg sessionBindMsg
	if err := gossh.Unmarshal(contents, &msg); err != nil {
		return agentforward.Binding{}, fmt.Errorf("%w: malformed", agentforward.ErrBindingRefused)
	}
	hostKey, err := gossh.ParsePublicKey(msg.HostKey)
	if err != nil {
		return agentforward.Binding{}, fmt.Errorf("%w: host key", agentforward.ErrBindingRefused)
	}
	var sig gossh.Signature
	if err := gossh.Unmarshal(msg.Signature, &sig); err != nil {
		return agentforward.Binding{}, fmt.Errorf("%w: signature encoding", agentforward.ErrBindingRefused)
	}
	// SHA-1 RSA proves too little to show a user as verified; the client's own crypto policy
	// refuses it for host keys, and so does this.
	if sig.Format == gossh.KeyAlgoRSA {
		return agentforward.Binding{}, fmt.Errorf("%w: sha-1 signature", agentforward.ErrBindingRefused)
	}
	if err := hostKey.Verify(msg.SessionID, &sig); err != nil {
		return agentforward.Binding{}, fmt.Errorf("%w: signature does not verify", agentforward.ErrBindingRefused)
	}
	return agentforward.Binding{
		SessionID:          msg.SessionID,
		HostKey:            msg.HostKey,
		HostKeyFingerprint: gossh.FingerprintSHA256(plainKey(hostKey)),
		Forwarding:         msg.Forwarding,
	}, nil
}

// plainKey is the key a host certificate certifies, which is what known_hosts lists and what the
// user recognises; for a plain key it is the key itself.
func plainKey(k gossh.PublicKey) gossh.PublicKey {
	if cert, ok := k.(*gossh.Certificate); ok {
		return cert.Key
	}
	return k
}
