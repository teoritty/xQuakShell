package wails

import (
	"fmt"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/usecase"
)

// The user's decisions about the identity of a remote peer.
//
// Separate from handlers_sessions.go, where the host key handler lives and serves the SSH path.
// The second dialog deliberately shares neither a file nor a state with it - editing one must not
// disturb the other.

// SetPeerTrustService wires the peer trust service after composition.
//
// A setter rather than a constructor argument: the service needs the session manager, which is
// itself created inside NewAppAPI. The same order as the rest of the plugin wiring.
func (a *AppAPI) SetPeerTrustService(svc *usecase.PeerTrustService) {
	a.peerTrust = svc
	if svc != nil {
		svc.SetPromptNotifier(a.onPeerTrustRequired)
	}
}

// onPeerTrustRequired tells the frontend a session is waiting on a trust decision.
//
// The fingerprint goes out, not the material: the first is what a human must see, and the second
// already sits on the session and will be written from there. Bytes the UI has no use for are not
// sent to it.
func (a *AppAPI) onPeerTrustRequired(sessionID, subject, fingerprint string, mismatch bool) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventPeerTrustRequired, map[string]interface{}{
		"sessionId":   sessionID,
		"subject":     subject,
		"fingerprint": fingerprint,
		"mismatch":    mismatch,
	})
}

// ResolvePeerTrust records the user's decision about the remote peer this session is waiting on.
// action is "trust" or "reject".
//
// The subject and the material come from the session's own pending decision rather than from the
// arguments - the same rule as ResolveHostKey, and for the same reason: a caller is not entitled
// to name what it is trusting.
//
// fingerprint is the exception, and it flows the other way: it is what the dialog displayed, sent
// back so the backend can refuse an answer that no longer matches the pending question. Without
// it, a decision made about one question could be applied to another.
func (a *AppAPI) ResolvePeerTrust(sessionID, action, fingerprint string) error {
	if a.peerTrust == nil {
		return fmt.Errorf("peer trust service unavailable")
	}
	scope, ok := a.scopeForSession(sessionID)
	if !ok {
		return fmt.Errorf("session %s: no owning plugin", sessionID)
	}
	return a.peerTrust.Resolve(a.reqCtx(), scope, sessionID, action, fingerprint)
}

// GetPeerTrust lists every remote identity the user has confirmed.
func (a *AppAPI) GetPeerTrust() ([]PeerTrustDTO, error) {
	if a.peerTrust == nil {
		return nil, fmt.Errorf("peer trust service unavailable")
	}
	entries, err := a.peerTrust.ListTrustedPeers()
	if err != nil {
		return nil, err
	}
	return PeerTrustToDTO(entries), nil
}

// RemovePeerTrust forgets one confirmed identity, so the next connection asks again.
//
// Scope and subject are named by the caller here, unlike everywhere else in this file, and that is
// correct: this is the user acting on a list the core itself produced, not a plugin naming a
// resource. The pair is a coordinate in that list; a wrong one deletes nothing.
func (a *AppAPI) RemovePeerTrust(scope, subject string) error {
	if a.peerTrust == nil {
		return fmt.Errorf("peer trust service unavailable")
	}
	return a.peerTrust.Revoke(a.reqCtx(), scope, subject)
}

// scopeForSession answers which plugin owns a session, which is the scope its trust is recorded
// under.
//
// Determined by the core from the session binding: the frontend has no right to name someone
// else's scope. The lookup reads the session registry through the session manager - the same
// binding the RPC layer authorized the plugin's original question against.
func (a *AppAPI) scopeForSession(sessionID string) (string, bool) {
	if a.sessions == nil {
		return "", false
	}
	return a.sessions.PluginIDForSession(sessionID)
}
