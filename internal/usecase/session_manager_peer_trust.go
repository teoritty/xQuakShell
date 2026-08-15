package usecase

import (
	"context"

	"xquakshell/internal/domain"
)

// Peer trust delegates. A file of its own rather than another block in the general forwarding
// list: they exist alongside PeerTrustService and change with it, while session_manager.go changes
// for entirely different reasons.
//
// RetrySession belongs to this set too, but it is already forwarded next to the SSH path and needs
// no second declaration.

func (m *SessionManager) BeginPeerTrustPrompt(sessionID string, prompt *PeerTrustPrompt) error {
	return m.lifecycle.BeginPeerTrustPrompt(sessionID, prompt)
}

func (m *SessionManager) GetPeerTrustPrompt(sessionID string) (*PeerTrustPrompt, error) {
	return m.lifecycle.GetPeerTrustPrompt(sessionID)
}

func (m *SessionManager) RejectPeerTrust(sessionID string) error {
	return m.lifecycle.RejectPeerTrust(sessionID)
}

func (m *SessionManager) ConnectionRecordForSession(ctx context.Context, sessionID string) (domain.Connection, error) {
	return m.lifecycle.ConnectionRecordForSession(ctx, sessionID)
}

// The manager is what reaches presentation, and the trust service is given exactly it. A mismatch
// has to be a build failure rather than a runtime refusal.
var _ PeerTrustSessions = (*SessionManager)(nil)
