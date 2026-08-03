package plugin

import "strings"

// RequiresSessionScopedProcess reports whether host IPC must target a session-scoped OS process (ADR-003).
func (m Manifest) RequiresSessionScopedProcess() bool {
	return m.EffectiveIsolation() == IsolationPerSession
}

// HostProcessScope returns the ProcessHost sessionID key for IPC to this plugin instance.
// Per-plugin isolation uses an empty key; per-session isolation requires a non-empty sessionID.
func (m Manifest) HostProcessScope(sessionID string) (string, error) {
	if !m.RequiresSessionScopedProcess() {
		return "", nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", ErrSessionScopeRequired
	}
	return sessionID, nil
}
