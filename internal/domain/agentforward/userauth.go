package agentforward

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	msgUserAuthRequest = 50
	serviceConnection  = "ssh-connection"
	methodPublicKey    = "publickey"
	// methodHostbound is what OpenSSH 8.9+ sends when the server offers it: the plain publickey
	// fields followed by the server's host key, so the signature commits to the destination.
	methodHostbound = "publickey-hostbound-v00@openssh.com"
)

const (
	// MaxSessionIDLen is the longest exchange hash SSH produces (SHA-512). Anything longer is not
	// a session identifier.
	MaxSessionIDLen = 64
	// MaxUserLen bounds the user name we show in the confirmation dialog. sshd itself refuses far
	// shorter names; the bound exists so the dialog cannot be filled with attacker text.
	MaxUserLen = 256
	// maxKeyBlobLen is generous for a 16384-bit RSA key and stops a request from carrying a blob
	// whose only purpose is size.
	maxKeyBlobLen = 16 * 1024
)

// UserAuthRequest is a parsed publickey login request: what a remote SSH client asks the agent to
// sign so it can authenticate to the next server.
type UserAuthRequest struct {
	SessionID []byte
	User      string
	Algorithm string
	KeyBlob   []byte
	// HostKey is the destination's host key blob when the request is the hostbound variant; nil
	// for a plain publickey request.
	HostKey []byte
}

// ParseUserAuthRequest accepts data only if it is exactly an SSH_MSG_USERAUTH_REQUEST for the
// publickey method, with the signature flag set, as RFC 4252 §7 lays it out.
//
// This is the rule that keeps a forwarded agent from being a signing oracle. A remote that can
// reach the agent socket can ask for a signature over any bytes; refusing everything that is not a
// login means it cannot obtain a signed git commit, a certificate, or a message of a protocol we
// have never heard of that happens to trust this key.
func ParseUserAuthRequest(data []byte) (UserAuthRequest, error) {
	r := reader{buf: data}
	var req UserAuthRequest
	req.SessionID = r.str()
	msgType := r.byte1()
	user := r.str()
	service := r.str()
	method := string(r.str())
	hasSignature := r.bool1()
	req.Algorithm = string(r.str())
	req.KeyBlob = r.str()
	if method == methodHostbound {
		req.HostKey = r.str()
	}

	switch {
	case !r.done():
		return UserAuthRequest{}, reject("malformed or trailing data")
	case msgType != msgUserAuthRequest:
		return UserAuthRequest{}, reject("message type %d", msgType)
	case string(service) != serviceConnection:
		return UserAuthRequest{}, reject("service %q", service)
	case method != methodPublicKey && method != methodHostbound:
		return UserAuthRequest{}, reject("method %q", method)
	case !hasSignature:
		return UserAuthRequest{}, reject("query without signature")
	case len(req.SessionID) == 0 || len(req.SessionID) > MaxSessionIDLen:
		return UserAuthRequest{}, reject("session id length %d", len(req.SessionID))
	case len(req.KeyBlob) == 0 || len(req.KeyBlob) > maxKeyBlobLen:
		return UserAuthRequest{}, reject("key blob length %d", len(req.KeyBlob))
	case method == methodHostbound && (len(req.HostKey) == 0 || len(req.HostKey) > maxKeyBlobLen):
		return UserAuthRequest{}, reject("host key length %d", len(req.HostKey))
	}
	if err := validateUser(user); err != nil {
		return UserAuthRequest{}, err
	}
	req.User = string(user)
	return req, nil
}

// validateUser keeps the user name displayable. It is shown in a confirmation dialog the user
// trusts, so a name carrying terminal escapes, newlines or invalid UTF-8 could rewrite what that
// dialog appears to say.
func validateUser(user []byte) error {
	if len(user) == 0 || len(user) > MaxUserLen || !utf8.Valid(user) {
		return reject("user name length %d or encoding", len(user))
	}
	for _, c := range string(user) {
		if unicode.IsControl(c) {
			return reject("control character in user name")
		}
	}
	return nil
}

// KeyFormat returns the algorithm name a public key blob starts with, e.g. "ssh-ed25519".
func KeyFormat(blob []byte) (string, error) {
	r := reader{buf: blob}
	name := r.str()
	if r.bad || len(name) == 0 {
		return "", reject("key blob without a format")
	}
	return string(name), nil
}

func reject(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrSignDataRejected}, args...)...)
}
