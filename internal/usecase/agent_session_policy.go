package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// agentSessionPolicy is the domain.AgentSignPolicy of one session. Every request is decided
// afresh against the vault as it is now: a key unticked or a connection edited mid-session takes
// effect at the next request, not at the next connect.
type agentSessionPolicy struct {
	svc     *AgentForwardingService
	ctx     context.Context
	info    AgentSessionInfo
	limiter *agentforward.RequestLimiter
	// confirmSlot holds one dialog per session at a time. A server that opens eight channels and
	// signs on each gets eight dialogs in a row, not eight stacked on top of one another.
	confirmSlot chan struct{}
}

var _ domain.AgentSignPolicy = (*agentSessionPolicy)(nil)

// forwardableKey is a vault key the session may sign with right now.
type forwardableKey struct {
	identity domain.SSHIdentity
	blob     []byte
}

func (p *agentSessionPolicy) alive() bool {
	return p.ctx.Err() == nil && p.svc.isLive(p)
}

// PublicKeys lists what the remote side may ask for, and nothing it may not.
func (p *agentSessionPolicy) PublicKeys(ctx context.Context) ([]domain.AgentPublicKey, error) {
	if !p.alive() {
		return nil, domain.ErrAgentSessionEnded
	}
	keys, _, err := p.forwardableKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentPublicKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, domain.AgentPublicKey{Blob: k.blob})
	}
	return out, nil
}

// Refuse audits an operation declined at the protocol level.
func (p *agentSessionPolicy) Refuse(ctx context.Context, operation string) {
	p.svc.cfg.Audit.Record(context.WithoutCancel(ctx), AgentAuditEvent{
		Event: AgentEventOpRefused, Session: p.info, Reason: operation,
	})
}

// Sign runs the pipeline of ADR-022 and audits its outcome either way.
func (p *agentSessionPolicy) Sign(ctx context.Context, req domain.AgentSignRequest) (*domain.Signature, error) {
	ev := AgentAuditEvent{Session: p.info}
	sig, err := p.sign(ctx, req, &ev)
	ev.Event = AgentEventSignAllowed
	if err != nil {
		ev.Event = AgentEventSignDenied
		ev.Reason = agentRefusalReason(err)
	}
	// The request context ends with the channel; a denial caused by the session closing must still
	// be written, so the audit write is detached from the cancellation but keeps its values.
	p.svc.cfg.Audit.Record(context.WithoutCancel(ctx), ev)
	return sig, err
}

func (p *agentSessionPolicy) sign(ctx context.Context, req domain.AgentSignRequest, ev *AgentAuditEvent) (*domain.Signature, error) {
	if !p.alive() {
		return nil, domain.ErrAgentSessionEnded
	}
	if err := p.limiter.Allow(); err != nil {
		return nil, err
	}
	login, err := agentforward.ParseUserAuthRequest(req.Data)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(login.KeyBlob, req.KeyBlob) {
		return nil, fmt.Errorf("%w: login names a different key than the request", agentforward.ErrSignDataRejected)
	}
	ev.RemoteUser = login.User
	key, cfg, err := p.keyFor(ctx, req.KeyBlob)
	if err != nil {
		return nil, err
	}
	ev.KeyFingerprint = key.identity.Fingerprint
	algorithm, err := signingAlgorithm(req, login)
	if err != nil {
		return nil, err
	}
	dest, verified, err := agentforward.Destination(login, req.Bindings)
	if err != nil {
		return nil, err
	}
	ev.Destination, ev.Verified = dest.HostKeyFingerprint, verified
	if err := p.approve(ctx, approvalRequest{key: key, login: login, dest: dest, verified: verified, cfg: cfg}); err != nil {
		return nil, err
	}
	return p.signWith(ctx, key, req.Data, algorithm)
}

func signingAlgorithm(req domain.AgentSignRequest, login agentforward.UserAuthRequest) (string, error) {
	format, err := agentforward.KeyFormat(req.KeyBlob)
	if err != nil {
		return "", err
	}
	return agentforward.SigningAlgorithm(format, login.Algorithm, req.Flags)
}

// keyFor finds the forwardable key with this blob. A blob that matches a vault key which is not
// forwardable gets the same refusal as one that matches nothing, so the remote side cannot probe
// which keys the vault holds.
func (p *agentSessionPolicy) keyFor(ctx context.Context, blob []byte) (forwardableKey, *agentforward.Config, error) {
	keys, cfg, err := p.forwardableKeys(ctx)
	if err != nil {
		return forwardableKey{}, nil, err
	}
	for _, k := range keys {
		if bytes.Equal(k.blob, blob) {
			return k, cfg, nil
		}
	}
	return forwardableKey{}, nil, domain.ErrAgentKeyNotForwardable
}

// forwardableKeys is the two-sided consent evaluated now: listed by the connection and flagged
// AllowForwarding on the key. A key whose cached public half cannot be decoded is skipped.
func (p *agentSessionPolicy) forwardableKeys(ctx context.Context) ([]forwardableKey, *agentforward.Config, error) {
	conn, err := p.svc.cfg.Connections.GetByID(ctx, p.info.ConnectionID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrAgentForwardingDisabled, err)
	}
	cfg := conn.AgentForwarding
	if !cfg.Active() {
		return nil, nil, domain.ErrAgentForwardingDisabled
	}
	keys := make([]forwardableKey, 0, len(cfg.IdentityIDs))
	for _, id := range cfg.IdentityIDs {
		identity, err := p.svc.cfg.Identities.Get(ctx, id)
		if err != nil || !identity.AllowForwarding {
			continue
		}
		blob, err := agentforward.AuthorizedKeyBlob(identity.PublicKey)
		if err != nil {
			continue
		}
		keys = append(keys, forwardableKey{identity: *identity, blob: blob})
	}
	return keys, cfg, nil
}

// signWith unlocks the key only now, after every check has passed, and confirms the signer is the
// key that was matched before signing with it.
func (p *agentSessionPolicy) signWith(ctx context.Context, key forwardableKey, data []byte, algorithm string) (*domain.Signature, error) {
	if p.svc.cfg.Signers == nil {
		return nil, fmt.Errorf("%w: no signer source", domain.ErrAgentSignDenied)
	}
	signer, err := p.svc.cfg.Signers.SignerFor(ctx, key.identity.ID)
	if err != nil {
		return nil, fmt.Errorf("agent forwarding: unlock key: %w", err)
	}
	if !bytes.Equal(signer.PublicKey().Marshal(), key.blob) {
		return nil, domain.ErrAgentKeyNotForwardable
	}
	algSigner, ok := signer.(domain.AlgorithmSigner)
	if !ok {
		return nil, fmt.Errorf("%w: key cannot choose its algorithm", agentforward.ErrAlgorithmRefused)
	}
	sig, err := algSigner.SignWithAlgorithm(rand.Reader, data, algorithm)
	if err != nil {
		return nil, fmt.Errorf("agent forwarding: sign: %w", err)
	}
	return sig, nil
}

// agentRefusalReason maps a refusal to a fixed audit token. The error text itself is never
// logged: some of it quotes remote input.
func agentRefusalReason(err error) string {
	reasons := []struct {
		target error
		reason string
	}{
		{domain.ErrAgentSessionEnded, "session-ended"},
		{agentforward.ErrRateLimited, "rate-limited"},
		{agentforward.ErrSignDataRejected, "not-a-login"},
		{agentforward.ErrAlgorithmRefused, "algorithm"},
		{agentforward.ErrHostKeyMismatch, "host-key-mismatch"},
		{domain.ErrAgentForwardingDisabled, "disabled"},
		{domain.ErrAgentKeyNotForwardable, "not-forwardable"},
		{domain.ErrAgentSignDenied, "denied"},
		{context.Canceled, "session-ended"},
	}
	for _, r := range reasons {
		if errors.Is(err, r.target) {
			return r.reason
		}
	}
	return "error"
}
