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

// SessionIOService owns PTY, RemoteFS, Exec, and SSH keepalive for active sessions.
//
// WHY THIS FILE/TYPE EXISTS (see ADR-009, docs/adr/009-session-manager-decomposition.md):
// Terminal IO and SFTP setup are session-runtime concerns separate from SSH auth,
// plugin protocol, and embed tunnels. This service reads/writes session fields only
// through SessionRegistry and must not contain lifecycle or plugin logic.
type SessionIOService struct {
	registry          *SessionRegistry
	vaultRepo         domain.VaultRepository
	ptyBridgeFactory  domain.PTYBridgeFactory
	sftpClientFactory domain.SFTPClientFactory
	onDisconnected    func(sessionID string)
}

// SessionIOServiceConfig configures SessionIOService.
type SessionIOServiceConfig struct {
	Registry          *SessionRegistry
	VaultRepo         domain.VaultRepository
	PTYBridgeFactory  domain.PTYBridgeFactory
	SFTPClientFactory domain.SFTPClientFactory
	OnDisconnected    func(sessionID string)
}

// NewSessionIOService creates a SessionIOService.
func NewSessionIOService(cfg SessionIOServiceConfig) *SessionIOService {
	return &SessionIOService{
		registry:          cfg.Registry,
		vaultRepo:         cfg.VaultRepo,
		ptyBridgeFactory:  cfg.PTYBridgeFactory,
		sftpClientFactory: cfg.SFTPClientFactory,
		onDisconnected:    cfg.OnDisconnected,
	}
}

// GetSSHClient returns the SSH client for a session.
func (s *SessionIOService) GetSSHClient(sessionID string) (domain.SSHClient, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.sshClient == nil {
		return nil, fmt.Errorf("session %s not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.sshClient, nil
}

// GetRemoteFS returns the remote filesystem for a session.
func (s *SessionIOService) GetRemoteFS(sessionID string) (domain.RemoteFS, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.remoteFS == nil {
		return nil, fmt.Errorf("session %s remote fs not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.remoteFS, nil
}

// GetPTYBridge returns the PTY bridge for a session.
func (s *SessionIOService) GetPTYBridge(sessionID string) (domain.TerminalPTYBridge, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	if entry.ptyBridge == nil {
		return nil, fmt.Errorf("session %s pty not ready: %w", sessionID, domain.ErrSessionNotFound)
	}
	return entry.ptyBridge, nil
}

// GetSessionContext returns the context for a session.
func (s *SessionIOService) GetSessionContext(sessionID string) (context.Context, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.ctx, nil
}

// SetRemoteFS stores the SFTP remote filesystem for a session.
func (s *SessionIOService) SetRemoteFS(sessionID string, fs domain.RemoteFS) {
	s.registry.Mutate(sessionID, func(entry *sessionEntry) {
		entry.remoteFS = fs
	})
}

// SetPTYBridge stores the PTY bridge for a session and applies any window size
// that the frontend requested before the bridge existed (resize/bridge-start race).
func (s *SessionIOService) SetPTYBridge(sessionID string, bridge domain.TerminalPTYBridge) {
	var cols, rows uint32
	s.registry.Mutate(sessionID, func(entry *sessionEntry) {
		entry.ptyBridge = bridge
		cols, rows = entry.ptyCols, entry.ptyRows
	})

	if bridge != nil && cols > 0 && rows > 0 {
		if err := bridge.Resize(cols, rows); err != nil {
			slog.Warn("pty resize failed on bridge set", "err", err)
		}
	}
}

// ResizeTerminal records the requested PTY window size and applies it if the
// bridge is ready. When the bridge has not started yet the size is buffered and
// applied later by SetPTYBridge, so an early resize is never lost.
func (s *SessionIOService) ResizeTerminal(sessionID string, cols, rows uint32) error {
	var bridge domain.TerminalPTYBridge
	if !s.registry.Mutate(sessionID, func(entry *sessionEntry) {
		entry.ptyCols = cols
		entry.ptyRows = rows
		bridge = entry.ptyBridge
	}) {
		return domain.ErrSessionNotFound
	}

	if bridge == nil {
		return nil
	}
	return bridge.Resize(cols, rows)
}

// InitSessionIO waits for the session to become ready.
//
// WHY THIS USES A CHANNEL, NOT A TICKER:
// This used to poll GetState() every 100ms for up to 30s. That adds up to
// 100ms of pure latency to every session start and wakes a goroutine 300
// times in the worst case for no reason. SessionRegistry now exposes a
// per-session "ready" broadcast; block on that instead of sleeping and
// re-checking.
func (s *SessionIOService) InitSessionIO(ctx context.Context, sessionID string) (<-chan []byte, string, error) {
	if s.ptyBridgeFactory == nil || s.sftpClientFactory == nil {
		return nil, "", nil
	}

	readyCh, err := s.registry.WaitReady(sessionID)
	if err != nil {
		return nil, "", nil
	}

	select {
	case <-readyCh:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}

	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, "", nil
	}
	info := entry.info
	if info.State == domain.SessionError || info.State == domain.SessionClosed {
		return nil, "", nil
	}
	if info.State != domain.SessionReady {
		return nil, "", nil
	}

	proto := info.Protocol
	if proto == "" {
		proto = domain.ProtocolSSH
	}
	if proto != domain.ProtocolSSH {
		return nil, "", nil
	}

	sshClient, err := s.GetSSHClient(sessionID)
	if err != nil {
		return nil, "", nil
	}

	sessionCtx, err := s.GetSessionContext(sessionID)
	if err != nil {
		return nil, "", nil
	}

	bridge := s.ptyBridgeFactory.NewBridge()
	outputCh, err := bridge.Start(sessionCtx, sshClient, domain.PTYOptions{
		Cols: 80, Rows: 24, Term: "xterm-256color",
	})
	if err != nil {
		return nil, "", fmt.Errorf("pty start: %w", err)
	}
	s.SetPTYBridge(sessionID, bridge)

	rateLimitKbps := 0
	if data, err := s.vaultRepo.GetData(); err == nil && data.Settings != nil {
		rateLimitKbps = data.Settings.Transfer.SpeedLimitKbps
	}

	remoteFS, err := s.sftpClientFactory.New(sshClient, rateLimitKbps)
	if err != nil {
		return outputCh, "", nil
	}
	s.SetRemoteFS(sessionID, remoteFS)

	initialPath := "/"
	if wd, err := remoteFS.GetWorkingDirectory(sessionCtx); err == nil && wd != "" {
		initialPath = wd
	}

	return outputCh, initialPath, nil
}

// Exec runs a command on the remote host via SSH and returns trimmed combined output.
func (s *SessionIOService) Exec(sessionID, cmd string) (string, error) {
	sshClient, err := s.GetSSHClient(sessionID)
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

// RunServerAlive sends periodic keepalive requests to detect connection loss.
func (s *SessionIOService) RunServerAlive(entry *sessionEntry) {
	var client domain.SSHClient
	s.registry.View(entry.info.SessionID, func(e *sessionEntry) {
		client = e.sshClient
	})
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
				if s.onDisconnected != nil {
					s.onDisconnected(entry.info.SessionID)
				}
				return
			}
		}
	}
}
