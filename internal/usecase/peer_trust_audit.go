package usecase

// The audit half of peer trust.
//
// A trust decision is the security event this whole mechanism exists to produce, and the plugin's
// RPC audit line does not carry it: that line records that a plugin asked, not what the human
// answered. An incident review needs the answer, when it was given, and against which fingerprint.
//
// The material is never part of an entry. A fingerprint identifies what was trusted well enough to
// investigate with, and an audit log is not a second copy of the vault.

// Actions a peer trust audit entry can carry. Named constants rather than literals at the call
// sites, because a typo in one of them produces a log nobody can filter on and nothing that fails.
const (
	peerTrustAuditPrompt   = "trust.prompt"
	peerTrustAuditDecision = "trust.decision"
	peerTrustAuditRevoke   = "trust.revoke"
)

// PeerTrustAuditEntry is one recordable moment in the life of a trust decision.
//
// A struct rather than a parameter list: five values of which three are strings would be a
// call-site ordering bug waiting to happen, and the function-parameter budget exists to say so.
type PeerTrustAuditEntry struct {
	// Action is one of the peerTrustAudit* constants.
	Action string
	// Scope is the plugin the trust belongs to. Empty for nothing - it always comes from a
	// session binding or from the entry being revoked.
	Scope string
	// SessionID is empty for a revocation: revoking is done from the manager screen, which is
	// not attached to any session.
	SessionID   string
	Subject     string
	Fingerprint string
	// Allowed is false for a refused question, a rejected identity and a failed write alike.
	// The Error field is what tells them apart.
	Allowed bool
	Error   string
}

// PeerTrustAuditFunc records one entry. Nil is a valid value and means no audit sink is wired yet.
type PeerTrustAuditFunc func(entry PeerTrustAuditEntry)

// SetAuditRecorder wires the audit sink.
//
// A setter for the same reason SetPromptNotifier is one: the writer is built in the composition
// root after this service exists.
func (s *PeerTrustService) SetAuditRecorder(record PeerTrustAuditFunc) {
	s.audit = record
}

// asAllowed and asDenied finish a half-built entry.
//
// A caller has the action, the scope and the subject long before it knows the outcome, so the
// entry is built once at the top and completed on whichever path is taken. That is also why
// recordAudit takes a struct: the alternative was one function of seven positional arguments, five
// of them strings, which is a call-site ordering bug with a schedule rather than a question of
// style.
func (e PeerTrustAuditEntry) asAllowed(fingerprint string) PeerTrustAuditEntry {
	e.Fingerprint = fingerprint
	e.Allowed = true
	return e
}

func (e PeerTrustAuditEntry) asDenied(fingerprint string, err error) PeerTrustAuditEntry {
	e.Fingerprint = fingerprint
	e.Allowed = false
	if err != nil {
		e.Error = err.Error()
	}
	return e
}

// recordAudit is the single place that reaches the sink, so a missing one is handled once.
func (s *PeerTrustService) recordAudit(entry PeerTrustAuditEntry) {
	if s == nil || s.audit == nil {
		return
	}
	s.audit(entry)
}
