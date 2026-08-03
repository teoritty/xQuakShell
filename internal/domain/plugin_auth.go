package domain

import "context"

// PluginAuthMethod identifies a manifest-declared auth method contributed by a plugin.
// Kind must be one of AuthProviderKindKeyboardInteractive / AuthProviderKindPublicKey.
type PluginAuthMethod struct {
	PluginID     string
	AuthMethodID string
	Kind         string
	// ConnectionID and Fields are runtime context from the connection being dialed (not manifest).
	ConnectionID string
	Fields       map[string]string
}

const (
	AuthProviderKindKeyboardInteractive = "keyboard-interactive"
	AuthProviderKindPublicKey           = "publickey"
)

// PluginAuthQuestion mirrors one keyboard-interactive question from the SSH server.
type PluginAuthQuestion struct {
	Text   string
	EchoOn bool
}

// PluginAuthChallenge is one keyboard-interactive round trip.
type PluginAuthChallenge struct {
	Name        string
	Instruction string
	Questions   []PluginAuthQuestion
}

// PluginAuthSignRequest asks a plugin-held key to sign opaque handshake data.
// DataToSign and the resulting signature are never persisted or logged verbatim
// anywhere in core (see security model).
type PluginAuthSignRequest struct {
	DataToSign []byte
	// Algorithms lists SSH public-key algorithm names the server will accept,
	// in preference order (e.g. "rsa-sha2-512", "ssh-ed25519").
	Algorithms []string
}

// PluginAuthSignResult carries signature bytes already in OpenSSH wire format,
// so infra/ssh can hand them to golang.org/x/crypto/ssh unmodified.
type PluginAuthSignResult struct {
	Signature       []byte
	SignatureFormat string
}

// PluginAuthProvider is implemented in usecase (backed by the plugin RPC transport)
// and consumed by usecase/ssh_connector.go to build a domain.AuthMethod via the
// AuthMethodBuilder factory (infra-implemented). Domain only defines the data shape.
type PluginAuthProvider interface {
	// Prepare is called once per auth attempt before the handshake. It lets the
	// plugin fetch/refresh a certificate from an external CA and returns the public
	// key/certificate blob (OpenSSH wire format) to advertise to the server.
	Prepare(ctx context.Context, attemptID string, method PluginAuthMethod) (publicKeyBlob []byte, err error)

	// AnswerChallenge handles one keyboard-interactive round trip. May be called
	// more than once per attempt (server can issue several rounds).
	AnswerChallenge(ctx context.Context, attemptID string, method PluginAuthMethod, challenge PluginAuthChallenge) (answers []string, err error)

	// Sign performs the actual signing operation. Called once per publickey
	// attempt (the ssh library may retry with a different algorithm).
	Sign(ctx context.Context, attemptID string, method PluginAuthMethod, req PluginAuthSignRequest) (PluginAuthSignResult, error)
}
