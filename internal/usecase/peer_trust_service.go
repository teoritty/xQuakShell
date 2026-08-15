package usecase

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"strconv"
	"strings"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// The port is the only way a plugin reaches this service.
var _ domainplugin.PeerTrustInboundPort = (*PeerTrustService)(nil)

// PeerTrustPrompt is what gets shown to the user, and what will be written if they agree.
//
// The material lives here rather than being supplied again at decision time: what is stored must
// be exactly what the human was shown. There must be no gap between the two that another value
// could fit into.
type PeerTrustPrompt struct {
	Subject     string
	Fingerprint string
	Material    []byte
	Mismatch    bool
}

// PeerTrustSessions is what the service needs from the session lifecycle.
type PeerTrustSessions interface {
	// ConnectionRecordForSession returns the connection record the session runs on. This is the
	// ONLY source of a trust subject.
	//
	// The context is real here: reading a connection goes to a repository, and a cancelled call
	// must not keep working.
	ConnectionRecordForSession(ctx context.Context, sessionID string) (domain.Connection, error)
	// BeginPeerTrustPrompt raises the question, or refuses when the session cannot take one.
	BeginPeerTrustPrompt(sessionID string, prompt *PeerTrustPrompt) error
	// GetPeerTrustPrompt returns a copy of the pending question, or nil.
	GetPeerTrustPrompt(sessionID string) (*PeerTrustPrompt, error)
	// RejectPeerTrust drops the pending question and fails the session.
	RejectPeerTrust(sessionID string) error
	RetrySession(ctx context.Context, sessionID string) error
}

// PeerTrustService answers one question: do we trust this remote side, and if not, it asks the
// human.
//
// The service knows no protocol. It knows a scope, a subject and bytes.
type PeerTrustService struct {
	repo     domain.PeerTrustRepository
	sessions PeerTrustSessions
	lookup   domain.ConnectionProtocolLookup
	notify   PeerTrustNotifier
	audit    PeerTrustAuditFunc
}

// PeerTrustNotifier is how the UI is woken up once a question exists.
//
// The material is not part of it. A human is shown the fingerprint; the bytes are only the
// storage's business, and there is no reason to let them out into presentation - so they are not.
type PeerTrustNotifier func(sessionID, subject, fingerprint string, mismatch bool)

// SetPromptNotifier wires the notification about a question appearing.
//
// A setter rather than a constructor argument: the notifier is presentation, which is built after
// the service. Its absence breaks nothing - the question still sits on the session, and its state
// already changed to trust-required.
func (s *PeerTrustService) SetPromptNotifier(notify PeerTrustNotifier) {
	s.notify = notify
}

// NewPeerTrustService creates the peer trust service.
//
// lookup is the same protocol registry the core computes a port from for session.connect. It is
// here for exactly that: a trust subject must name the address the plugin was actually sent to,
// or trust is recorded against one node and checked against another.
func NewPeerTrustService(
	repo domain.PeerTrustRepository,
	sessions PeerTrustSessions,
	lookup domain.ConnectionProtocolLookup,
) *PeerTrustService {
	return &PeerTrustService{repo: repo, sessions: sessions, lookup: lookup}
}

// VerifyPeer implements domainplugin.PeerTrustInboundPort.
//
// pluginID is supplied by the RPC layer from the process binding - a plugin never sends its own
// name in the parameters. Here it becomes the trust scope: this is the single place where the port
// name and the scope name meet, and the translation happens once instead of in every caller.
func (s *PeerTrustService) VerifyPeer(ctx context.Context, pluginID, sessionID, subject string, material []byte) (bool, error) {
	return s.Verify(ctx, pluginID, sessionID, subject, material)
}

// Verify checks observed material against confirmed material.
//
// scope is passed by the caller from the session binding, not from the RPC parameters: the
// contract has no "scope" field at all, so a plugin cannot name someone else's.
//
// claimedSubject comes from the plugin. It is checked against the connection record and then NOT
// used: both the dialog and the storage get the core's version. The point of the field is that a
// disagreement becomes visible. If a plugin believes it is talking to one peer while it was sent
// to another, that is either a defect or an attack, and the connection is refused instead of
// quietly succeeding.
func (s *PeerTrustService) Verify(ctx context.Context, scope, sessionID, claimedSubject string, material []byte) (bool, error) {
	if len(material) == 0 {
		return false, domain.ErrPeerMaterialEmpty
	}
	if len(material) > domain.MaxPeerTrustMaterial {
		return false, domain.ErrPeerMaterialTooLarge
	}

	subject, err := s.subjectFor(ctx, sessionID)
	if err != nil {
		return false, err
	}
	question := PeerTrustAuditEntry{
		Action:    peerTrustAuditPrompt,
		Scope:     scope,
		SessionID: sessionID,
		Subject:   subject,
	}
	// Refused before any write and before any dialog: there is nothing to show a human about a
	// peer we have already stopped understanding.
	if claimedSubject != subject {
		err := fmt.Errorf("peer trust: plugin claims %q, session connects to %q", claimedSubject, subject)
		s.recordAudit(question.asDenied("", err))
		return false, err
	}

	known, err := s.repo.Find(scope, subject)
	if err != nil {
		return false, err
	}
	if known != nil && subtle.ConstantTimeCompare(known.Material, material) == 1 {
		return true, nil
	}

	prompt := &PeerTrustPrompt{
		Subject:     subject,
		Fingerprint: domain.PeerFingerprint(material),
		Material:    append([]byte(nil), material...),
		// A known subject with different material is a change, not a first meeting. The user
		// needs different words because the action is different.
		Mismatch: known != nil,
	}
	if err := s.sessions.BeginPeerTrustPrompt(sessionID, prompt); err != nil {
		s.recordAudit(question.asDenied(prompt.Fingerprint, err))
		return false, err
	}
	s.recordAudit(question.asAllowed(prompt.Fingerprint))
	if s.notify != nil {
		s.notify(sessionID, prompt.Subject, prompt.Fingerprint, prompt.Mismatch)
	}
	return false, nil
}

// Resolve records the user's decision.
//
// action is "trust" or "reject", and nothing else. The subject and the material come from the
// pending question rather than from the arguments, and a session with nothing pending is refused.
// Otherwise this call would be a way to write trust outside a check the core itself started -
// exactly the defect ResolveHostKey was cured of.
//
// fingerprint is what the caller displayed, echoed back. It must name the question that is
// actually pending: an answer that raced a replaced prompt is refused rather than applied to
// whatever is pending now. BeginPeerTrustPrompt already refuses the replacement, so this is the
// second lock on the same door - and the cheaper one to keep correct, because it compares the very
// string the user was looking at.
func (s *PeerTrustService) Resolve(ctx context.Context, scope, sessionID, action, fingerprint string) error {
	pending, err := s.sessions.GetPeerTrustPrompt(sessionID)
	if err != nil {
		return err
	}
	if pending == nil {
		return fmt.Errorf("session %s: %w", sessionID, domain.ErrPeerTrustNoPending)
	}
	if fingerprint == "" || fingerprint != pending.Fingerprint {
		return fmt.Errorf("session %s: %w", sessionID, domain.ErrPeerTrustPromptStale)
	}

	decision := PeerTrustAuditEntry{
		Action:    peerTrustAuditDecision,
		Scope:     scope,
		SessionID: sessionID,
		Subject:   pending.Subject,
	}
	switch action {
	case "trust":
		entry := domain.PeerTrustEntry{
			Scope:    scope,
			Subject:  pending.Subject,
			Material: pending.Material,
		}
		if err := s.repo.Put(ctx, entry); err != nil {
			s.recordAudit(decision.asDenied(fingerprint, err))
			return fmt.Errorf("store peer trust: %w", err)
		}
		s.recordAudit(decision.asAllowed(fingerprint))
		// The retry clears the pending question as part of the transition back to connecting,
		// so nothing clears it here: two owners of that field is how they drift apart.
		return s.sessions.RetrySession(ctx, sessionID)
	case "reject":
		s.recordAudit(decision.asDenied(fingerprint, nil))
		return s.sessions.RejectPeerTrust(sessionID)
	default:
		// The question is NOT cleared: an unknown action is a caller defect, and there is no
		// reason to lose a decision already on screen because of one.
		return fmt.Errorf("unknown peer trust action %q", action)
	}
}

// subjectFor derives the subject from the connection record.
//
// The port is the effective one, from the same call the core computes the port for session.connect
// with (plugin_session_bridge.go). A private copy of that rule here would mean the two drift one
// day, and trust would be recorded against an address the plugin was not sent to.
func (s *PeerTrustService) subjectFor(ctx context.Context, sessionID string) (string, error) {
	conn, err := s.sessions.ConnectionRecordForSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(conn.Host)
	if host == "" {
		return "", fmt.Errorf("session %s: connection has no host", sessionID)
	}
	port := conn.EffectivePort(s.lookup)
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("session %s: connection has no usable port", sessionID)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
