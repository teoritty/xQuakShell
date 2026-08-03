package usecase

import (
	"context"

	"xquakshell/internal/domain"
)

// GetSSHClient returns the SSH client for a session.
func (m *SessionManager) GetSSHClient(sessionID string) (domain.SSHClient, error) {
	return m.io.GetSSHClient(sessionID)
}

// GetRemoteFS returns the remote filesystem for a session.
func (m *SessionManager) GetRemoteFS(sessionID string) (domain.RemoteFS, error) {
	return m.io.GetRemoteFS(sessionID)
}

// GetPTYBridge returns the PTY bridge for a session.
func (m *SessionManager) GetPTYBridge(sessionID string) (domain.TerminalPTYBridge, error) {
	return m.io.GetPTYBridge(sessionID)
}

// GetSessionContext returns the context for a session.
func (m *SessionManager) GetSessionContext(sessionID string) (context.Context, error) {
	return m.io.GetSessionContext(sessionID)
}

// SetRemoteFS stores the SFTP remote filesystem for a session.
func (m *SessionManager) SetRemoteFS(sessionID string, fs domain.RemoteFS) {
	m.io.SetRemoteFS(sessionID, fs)
}

// SetPTYBridge stores the PTY bridge for a session.
func (m *SessionManager) SetPTYBridge(sessionID string, bridge domain.TerminalPTYBridge) {
	m.io.SetPTYBridge(sessionID, bridge)
}

// ResizeTerminal records and applies PTY window size for a session.
func (m *SessionManager) ResizeTerminal(sessionID string, cols, rows uint32) error {
	return m.io.ResizeTerminal(sessionID, cols, rows)
}

// InitSessionIO waits until the SSH session is ready, then initialises PTY and SFTP.
func (m *SessionManager) InitSessionIO(ctx context.Context, sessionID string) (<-chan []byte, string, error) {
	return m.io.InitSessionIO(ctx, sessionID)
}

// Exec runs a command on the remote host via SSH.
func (m *SessionManager) Exec(sessionID, cmd string) (string, error) {
	return m.io.Exec(sessionID, cmd)
}

// runServerAlive sends periodic keepalive requests to detect connection loss.
func (m *SessionManager) runServerAlive(entry *sessionEntry) {
	m.io.RunServerAlive(entry)
}
