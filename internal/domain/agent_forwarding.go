package domain

import (
	"context"
	"errors"

	"golang.org/x/crypto/ssh"

	"xquakshell/internal/domain/agentforward"
)

// Errors a forwarded agent's policy returns. Each names one refusal so the audit log and the
// tests can tell them apart; none carries the data that was refused.
var (
	ErrAgentSessionEnded       = errors.New("agent forwarding: session has ended")
	ErrAgentForwardingDisabled = errors.New("agent forwarding: disabled for this connection")
	ErrAgentKeyNotForwardable  = errors.New("agent forwarding: key is not forwarded to this connection")
	ErrAgentSignDenied         = errors.New("agent forwarding: signature denied")
	ErrAgentOperationRefused   = errors.New("agent forwarding: operation refused")
	ErrNoPendingAgentConfirm   = errors.New("agent forwarding: no such pending confirmation")
)

// Signature is a type alias for ssh.Signature, so usecase can return signatures through the agent
// port without importing golang.org/x/crypto/ssh.
type Signature = ssh.Signature

// AlgorithmSigner is a type alias for ssh.AlgorithmSigner: a signer that can be told which
// algorithm to use, which an RSA key needs to sign with SHA-2.
type AlgorithmSigner = ssh.AlgorithmSigner

// AgentPublicKey is one key a forwarded agent lists. Comment is always empty: a key's label in the
// vault ("prod deploy key") is the user's, and listing it would tell every server the agent is
// forwarded to what else the user can reach.
type AgentPublicKey struct {
	Blob []byte
}

// AgentSignRequest is one signature request as a forwarded agent channel received it.
type AgentSignRequest struct {
	KeyBlob []byte
	Data    []byte
	Flags   agentforward.Flags
	// Bindings are the verified session-binds of the channel the request arrived on.
	Bindings *agentforward.Bindings
}

// AgentSignPolicy decides every request a forwarded agent receives on one session. The
// infrastructure that speaks the agent protocol owns no rule of its own: what to list, what to
// sign and what to refuse are all asked of this port.
type AgentSignPolicy interface {
	// PublicKeys lists the keys forwardable right now.
	PublicKeys(ctx context.Context) ([]AgentPublicKey, error)
	// Sign returns a signature or the refusal that prevented one.
	Sign(ctx context.Context, req AgentSignRequest) (*ssh.Signature, error)
	// Refuse records an operation the agent declined without asking anyone, such as an attempt
	// to add a key or a forged session-bind. operation is a fixed token, never remote input.
	Refuse(ctx context.Context, operation string)
}
