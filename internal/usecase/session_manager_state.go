package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

func (m *SessionManager) updateState(entry *sessionEntry, state domain.SessionState, errMsg string) {
	sessionID := entry.info.SessionID
	var info domain.ConnectionSession
	if !m.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.info.State = state
		e.info.ErrorMessage = errMsg
		info = e.info
		e.signalReadyIfTerminal(state)
	}) {
		return
	}
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
	entry, ok := m.registry.Get(sessionID)
	if !ok {
		return domain.ErrSessionNotFound
	}
	if entry.info.State != domain.SessionHostKeyRequired {
		return fmt.Errorf("session %s not in hostkey-required state", sessionID)
	}
	var info domain.ConnectionSession
	m.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.info.State = domain.SessionConnecting
		e.info.ErrorMessage = ""
		e.hostKeyInfo = nil
		info = e.info
	})
	connID := entry.connectionID

	m.notifyStateChange(info)

	conn, err := m.connRepo.GetByID(ctx, connID)
	if err != nil {
		slog.Error("retry session: load connection failed", "sessionID", sessionID, "err", err)
		m.updateState(entry, domain.SessionError, "Connection not found")
		return nil
	}

	if err := conn.ValidateForConnect(); err != nil {
		slog.Error("retry session: invalid connection", "sessionID", sessionID, "err", err)
		m.updateState(entry, domain.SessionError, "Invalid connection configuration")
		return nil
	}

	safego.GoNamed("session.reconnect", func() { m.connectSession(entry, conn) })
	return nil
}

// NotifySessionDisconnected is called when the terminal output stream closes (e.g. SSH connection lost).
// Updates the session state to SessionError so the UI can show "Connection lost" and offer Reconnect.
func (m *SessionManager) NotifySessionDisconnected(sessionID string) {
	entry, ok := m.registry.Get(sessionID)
	if !ok {
		return
	}
	if entry.info.State != domain.SessionReady {
		return
	}
	m.updateState(entry, domain.SessionError, "Connection lost")
}

// GetHostKeyInfo returns the pending host key info for a session, if any.
func (m *SessionManager) GetHostKeyInfo(sessionID string) (*domain.HostKeyInfo, error) {
	entry, ok := m.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.hostKeyInfo, nil
}
