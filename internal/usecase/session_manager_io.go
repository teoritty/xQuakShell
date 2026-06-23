package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ssh-client/internal/domain"
)

const serverAliveInterval = 30 * time.Second

// GetSSHClient returns the SSH client for a session.
func (m *SessionManager) GetSSHClient(sessionID string) (domain.SSHClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.sshClient == nil {
		return nil, fmt.Errorf("session %s not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.sshClient, nil
}

// GetRemoteFS returns the remote filesystem for a session.
func (m *SessionManager) GetRemoteFS(sessionID string) (domain.RemoteFS, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.remoteFS == nil {
		return nil, fmt.Errorf("session %s remote fs not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.remoteFS, nil
}

// GetPTYBridge returns the PTY bridge for a session.
func (m *SessionManager) GetPTYBridge(sessionID string) (domain.TerminalPTYBridge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.ptyBridge == nil {
		return nil, fmt.Errorf("session %s pty not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.ptyBridge, nil
}

// GetSessionContext returns the context for a session.
func (m *SessionManager) GetSessionContext(sessionID string) (context.Context, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.ctx, nil
}

// SetRemoteFS stores the SFTP remote filesystem for a session.
func (m *SessionManager) SetRemoteFS(sessionID string, fs domain.RemoteFS) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[sessionID]; ok {
		entry.remoteFS = fs
	}
}

// SetPTYBridge stores the PTY bridge for a session and applies any window size
// that the frontend requested before the bridge existed (resize/bridge-start race).
func (m *SessionManager) SetPTYBridge(sessionID string, bridge domain.TerminalPTYBridge) {
	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	entry.ptyBridge = bridge
	cols, rows := entry.ptyCols, entry.ptyRows
	m.mu.Unlock()

	if bridge != nil && cols > 0 && rows > 0 {
		if err := bridge.Resize(cols, rows); err != nil {
			slog.Warn("pty resize failed on bridge set", "err", err)
		}
	}
}

// ResizeTerminal records the requested PTY window size and applies it if the
// bridge is ready. When the bridge has not started yet the size is buffered and
// applied later by SetPTYBridge, so an early resize is never lost.
func (m *SessionManager) ResizeTerminal(sessionID string, cols, rows uint32) error {
	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domain.ErrSessionNotFound
	}
	entry.ptyCols = cols
	entry.ptyRows = rows
	bridge := entry.ptyBridge
	m.mu.Unlock()

	if bridge == nil {
		return nil
	}
	return bridge.Resize(cols, rows)
}

// InitSessionIO waits until the SSH session is ready, then initialises the PTY and SFTP subsystems.
// It returns the terminal output channel and the initial remote working directory.
// For non-SSH sessions (plugin connectors) this is a no-op and returns nil, "", nil.
func (m *SessionManager) InitSessionIO(ctx context.Context, sessionID string) (<-chan []byte, string, error) {
	if m.ptyBridgeFactory == nil || m.sftpClientFactory == nil {
		return nil, "", nil
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var info domain.ConnectionSession
	for i := 0; i < 300; i++ {
		<-ticker.C
		var err error
		info, err = m.GetState(sessionID)
		if err != nil || info.State == domain.SessionError || info.State == domain.SessionClosed {
			return nil, "", nil
		}
		if info.State == domain.SessionReady {
			break
		}
	}

	proto := info.Protocol
	if proto == "" {
		proto = domain.ProtocolSSH
	}
	if proto != domain.ProtocolSSH {
		return nil, "", nil
	}

	sshClient, err := m.GetSSHClient(sessionID)
	if err != nil {
		return nil, "", nil
	}

	sessionCtx, err := m.GetSessionContext(sessionID)
	if err != nil {
		return nil, "", nil
	}

	bridge := m.ptyBridgeFactory.NewBridge()
	outputCh, err := bridge.Start(sessionCtx, sshClient, domain.PTYOptions{
		Cols: 80, Rows: 24, Term: "xterm-256color",
	})
	if err != nil {
		return nil, "", fmt.Errorf("pty start: %w", err)
	}
	m.SetPTYBridge(sessionID, bridge)

	rateLimitKbps := 0
	if data, err := m.vaultRepo.GetData(); err == nil && data.Settings != nil {
		rateLimitKbps = data.Settings.Transfer.SpeedLimitKbps
	}

	remoteFS, err := m.sftpClientFactory.New(sshClient, rateLimitKbps)
	if err != nil {
		return outputCh, "", nil
	}
	m.SetRemoteFS(sessionID, remoteFS)

	initialPath := "/"
	if wd, err := remoteFS.GetWorkingDirectory(sessionCtx); err == nil && wd != "" {
		initialPath = wd
	}

	return outputCh, initialPath, nil
}

// Exec runs a command on the remote host via SSH and returns trimmed combined output.
func (m *SessionManager) Exec(sessionID, cmd string) (string, error) {
	sshClient, err := m.GetSSHClient(sessionID)
	if err != nil {
		return "", err
	}
	session, err := sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runServerAlive sends periodic keepalive requests to detect connection loss.
// The SSH client reference is captured once under the read-lock to avoid a race with CloseSession.
func (m *SessionManager) runServerAlive(entry *sessionEntry) {
	m.mu.RLock()
	client := entry.sshClient
	m.mu.RUnlock()
	if client == nil {
		return
	}

	ticker := time.NewTicker(serverAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-entry.ctx.Done():
			return
		case <-ticker.C:
			if err := client.KeepAlive(); err != nil {
				m.NotifySessionDisconnected(entry.info.SessionID)
				return
			}
		}
	}
}
