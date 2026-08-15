package usecase

import (
	"bytes"
	"context"

	"xquakshell/internal/domain"
)

// The half of the session lifecycle that owns waiting on a trust decision.
//
// A file of its own rather than a section of session_lifecycle_service.go: that file has a size
// budget it may only shrink, and, more to the point, there is one reason to change this code -
// how a session holds and shows a pending decision.

// BeginPeerTrustPrompt puts a question in front of the user, or refuses to.
//
// Everything that decides the outcome is read and written inside ONE Mutate. Split across a Get
// and a later write, the check would be advisory: two plugin calls can interleave between them,
// and the whole point of the check is that they cannot.
//
// Three refusals, and each closes a concrete hole:
//
//   - A state other than connecting or trust-required. Without it a plugin could drag its own
//     established session back into trust-required at any moment.
//   - A pending question with different material. This is the consent-integrity case: a prompt
//     that could be overwritten would let a plugin show a harmless fingerprint, wait for the
//     dialog, and swap the material underneath it - the user clicks "trust" on one value and
//     stores another.
//   - No such session.
//
// The same material re-asked is not a refusal: a plugin retrying its handshake must not be
// punished for it, and nothing about the question changed.
//
// The state change happens after the Mutate returns, because updateState takes the same lock and
// sync.RWMutex is not reentrant.
func (s *SessionLifecycleService) BeginPeerTrustPrompt(sessionID string, prompt *PeerTrustPrompt) error {
	if prompt == nil {
		return domain.ErrPeerMaterialEmpty
	}
	var (
		entry     *sessionEntry
		fresh     bool
		promptErr error
	)
	if !s.registry.Mutate(sessionID, func(e *sessionEntry) {
		entry = e
		if e.info.State != domain.SessionConnecting && e.info.State != domain.SessionTrustRequired {
			promptErr = domain.ErrPeerTrustNotWaiting
			return
		}
		if e.peerTrustPrompt != nil {
			if !bytes.Equal(e.peerTrustPrompt.Material, prompt.Material) {
				promptErr = domain.ErrPeerTrustPromptBusy
			}
			return
		}
		e.peerTrustPrompt = prompt
		fresh = true
	}) {
		return domain.ErrSessionNotFound
	}
	if promptErr != nil {
		return promptErr
	}
	if !fresh {
		return nil
	}
	msg := "Remote identity verification required"
	if prompt.Mismatch {
		msg = "Remote identity changed"
	}
	// Held together with the pending value on purpose. A session that waits for a decision but
	// looks like it is connecting leaves the user staring at an endless "connecting" instead of
	// the question.
	s.updateState(entry, domain.SessionTrustRequired, msg)
	return nil
}

// GetPeerTrustPrompt returns a copy of the pending decision, or nil.
//
// A copy, including the material: handing out the pointer would put a live registry field in the
// caller's hands, and every read of it afterwards would race the plugin-driven writer.
func (s *SessionLifecycleService) GetPeerTrustPrompt(sessionID string) (*PeerTrustPrompt, error) {
	var pending *PeerTrustPrompt
	if !s.registry.Read(sessionID, func(e *sessionEntry) {
		if e.peerTrustPrompt == nil {
			return
		}
		clone := *e.peerTrustPrompt
		clone.Material = append([]byte(nil), e.peerTrustPrompt.Material...)
		pending = &clone
	}) {
		return nil, domain.ErrSessionNotFound
	}
	return pending, nil
}

// RejectPeerTrust drops the pending decision and fails the session.
//
// Failing it is the point. Clearing the question alone would leave the session in trust-required
// forever, showing "Remote identity verification required" for a question the user already
// answered - and still accepting a retry, which is the one thing a refusal must not allow.
func (s *SessionLifecycleService) RejectPeerTrust(sessionID string) error {
	var entry *sessionEntry
	if !s.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.peerTrustPrompt = nil
		entry = e
	}) {
		return domain.ErrSessionNotFound
	}
	s.updateState(entry, domain.SessionError, "Remote identity was not trusted")
	return nil
}

// ConnectionRecordForSession returns the connection record the session was opened from.
//
// The name differs from SessionRegistry.ConnectionForSession deliberately: that one yields an id,
// this one the record itself. The same name on two receivers would read as the same thing.
//
// This is the only source of a trust subject: everything the core knows about it comes from here,
// never from the parameters of a plugin call.
func (s *SessionLifecycleService) ConnectionRecordForSession(ctx context.Context, sessionID string) (domain.Connection, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return domain.Connection{}, domain.ErrSessionNotFound
	}
	conn, err := s.connRepo.GetByID(ctx, entry.connectionID)
	if err != nil {
		return domain.Connection{}, err
	}
	if conn == nil {
		return domain.Connection{}, domain.ErrSessionNotFound
	}
	return *conn, nil
}

// The session lifecycle must satisfy the trust service's port.
//
// Checked at compile time rather than at wiring time: signatures that drifted apart would
// otherwise surface in the composition root, far from where they were changed.
var _ PeerTrustSessions = (*SessionLifecycleService)(nil)

// takeWaitingSessionForRetry moves a session out of waiting-for-a-decision back into connecting
// and yields what a retry needs.
//
// Here rather than in session_lifecycle_service.go for two reasons. That file is pinned by a
// budget and may only shrink. And on the merits: two admissible source states are exactly what the
// trust mechanism added, so the explanation belongs next to it.
//
// Both states are listed explicitly rather than folded into "any waiting state":
// SessionHostKeyRequired belongs to the SSH path, SessionTrustRequired to the plugin one. A retry
// from any other state is refused, because the whole meaning of waiting is that the connection
// does not proceed until the decision is made.
func (s *SessionLifecycleService) takeWaitingSessionForRetry(sessionID string) (domain.ConnectionSession, string, bool) {
	var info domain.ConnectionSession
	var connID string
	// Both forms of waiting are cleared the same way: reconnecting, a session must not drag
	// someone else's unanswered decision along with it.
	clearPending := func(e *sessionEntry) {
		e.info.ErrorMessage = ""
		e.hostKeyInfo = nil
		e.peerTrustPrompt = nil
		info = e.info
		connID = e.connectionID
	}
	for _, from := range []domain.SessionState{domain.SessionHostKeyRequired, domain.SessionTrustRequired} {
		if s.registry.CompareAndTransition(sessionID, from, domain.SessionConnecting, clearPending) {
			return info, connID, true
		}
	}
	return info, connID, false
}
