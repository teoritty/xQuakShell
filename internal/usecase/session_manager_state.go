package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"ssh-client/internal/domain"
)

func (m *SessionManager) updateState(entry *sessionEntry, state domain.SessionState, errMsg string) {
	m.mu.Lock()
	_, stillRegistered := m.sessions[entry.info.SessionID]
	if !stillRegistered {
		m.mu.Unlock()
		return
	}
	entry.info.State = state
	entry.info.ErrorMessage = errMsg
	info := entry.info
	m.mu.Unlock()
	m.notifyStateChange(info)
}

func (m *SessionManager) notifyStateChange(info domain.ConnectionSession) {
	if m.onStateChange != nil {
		m.onStateChange(info)
	}
}

// RetrySession re-attempts the SSH connection for a session in hostkey-required state.
// Called after the user has added/replaced the host key via the UI.
func (m *SessionManager) RetrySession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domain.ErrSessionNotFound
	}
	if entry.info.State != domain.SessionHostKeyRequired {
		m.mu.Unlock()
		return fmt.Errorf("session %s not in hostkey-required state", sessionID)
	}
	entry.info.State = domain.SessionConnecting
	entry.info.ErrorMessage = ""
	entry.hostKeyInfo = nil
	connID := entry.connectionID
	m.mu.Unlock()

	m.notifyStateChange(entry.info)

	conn, err := m.connRepo.GetByID(ctx, connID)
	if err != nil {
		slog.Error("retry session: load connection failed", "sessionID", sessionID, "err", err)
		m.updateState(entry, domain.SessionError, "Connection not found")
		return nil
	}

	go m.connectSession(entry, conn)
	return nil
}

// NotifySessionDisconnected is called when the terminal output stream closes (e.g. SSH connection lost).
// Updates the session state to SessionError so the UI can show "Connection lost" and offer Reconnect.
func (m *SessionManager) NotifySessionDisconnected(sessionID string) {
	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if entry.info.State != domain.SessionReady {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.updateState(entry, domain.SessionError, "Connection lost")
}

// GetHostKeyInfo returns the pending host key info for a session, if any.
func (m *SessionManager) GetHostKeyInfo(sessionID string) (*domain.HostKeyInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.hostKeyInfo, nil
}
