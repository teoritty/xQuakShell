package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"ssh-client/internal/domain"
)

// sessionEntry holds runtime state for a single session (tab).
type sessionEntry struct {
	info         domain.ConnectionSession
	ctx          context.Context
	cancel       context.CancelFunc
	sshClient    domain.SSHClient
	remoteFS     domain.RemoteFS
	ptyBridge    domain.TerminalPTYBridge
	ptyCols      uint32
	ptyRows      uint32
	hostKeyInfo  *domain.HostKeyInfo
	connectionID string
}

// StateChangeFunc is called whenever a session transitions to a new state.
type StateChangeFunc func(session domain.ConnectionSession)

// PassphraseRequestFunc is called when an encrypted key needs a passphrase.
// The implementation should prompt the user and return the passphrase or an error.
type PassphraseRequestFunc func(identityID, comment string) (string, error)

// HostKeyRequestFunc is called when a host key decision is needed from the user.
// The sessionID identifies the session, info carries the key details.
type HostKeyRequestFunc func(sessionID string, info domain.HostKeyInfo)

// OnStreamReadyFunc is called when a stream-based plugin connector has started
// the terminal output. The API uses this to begin streaming to the frontend.
type OnStreamReadyFunc func(sessionID string, outputCh <-chan []byte)

// SessionManager manages the lifecycle of parallel sessions (tabs) for all protocols.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry

	connRepo          domain.ConnectionRepository
	vaultRepo         domain.VaultRepository
	identRepo         domain.IdentityRepository
	passwordRepo      domain.PasswordRepository
	knownHosts        domain.KnownHostsRepository
	sshFactory        domain.SSHClientFactory
	passphraseCache   domain.PassphraseCache
	hostKeyCB         domain.HostKeyCallbackBuilder
	jumpTransport     domain.JumpTransportBuilder
	keySigner         domain.PrivateKeySignerFactory
	ptyBridgeFactory  domain.PTYBridgeFactory
	sftpClientFactory domain.SFTPClientFactory
	connectors        map[string]domain.SessionConnector

	onStateChange  StateChangeFunc
	onStreamReady  OnStreamReadyFunc
	passphraseReq  PassphraseRequestFunc
	hostKeyRequest HostKeyRequestFunc
}

// SessionManagerConfig holds dependencies for creating a SessionManager.
type SessionManagerConfig struct {
	ConnRepo                domain.ConnectionRepository
	VaultRepo               domain.VaultRepository
	IdentRepo               domain.IdentityRepository
	PasswordRepo            domain.PasswordRepository
	KnownHosts              domain.KnownHostsRepository
	SSHFactory              domain.SSHClientFactory
	PassphraseCache         domain.PassphraseCache
	HostKeyCallbackBuilder  domain.HostKeyCallbackBuilder
	JumpTransportBuilder    domain.JumpTransportBuilder
	PrivateKeySignerFactory domain.PrivateKeySignerFactory
	PTYBridgeFactory        domain.PTYBridgeFactory
	SFTPClientFactory       domain.SFTPClientFactory
	Connectors              []domain.SessionConnector
	OnStateChange           StateChangeFunc
	OnStreamReady           OnStreamReadyFunc
	PassphraseReq           PassphraseRequestFunc
	HostKeyRequest          HostKeyRequestFunc
}

// NewSessionManager creates a SessionManager with the given dependencies.
func NewSessionManager(cfg SessionManagerConfig) *SessionManager {
	connectors := make(map[string]domain.SessionConnector)
	for _, c := range cfg.Connectors {
		connectors[c.Protocol()] = c
	}
	return &SessionManager{
		sessions:          make(map[string]*sessionEntry),
		connRepo:          cfg.ConnRepo,
		vaultRepo:         cfg.VaultRepo,
		identRepo:         cfg.IdentRepo,
		passwordRepo:      cfg.PasswordRepo,
		knownHosts:        cfg.KnownHosts,
		sshFactory:        cfg.SSHFactory,
		passphraseCache:   cfg.PassphraseCache,
		hostKeyCB:         cfg.HostKeyCallbackBuilder,
		jumpTransport:     cfg.JumpTransportBuilder,
		keySigner:         cfg.PrivateKeySignerFactory,
		ptyBridgeFactory:  cfg.PTYBridgeFactory,
		sftpClientFactory: cfg.SFTPClientFactory,
		connectors:        connectors,
		onStateChange:     cfg.OnStateChange,
		onStreamReady:     cfg.OnStreamReady,
		passphraseReq:     cfg.PassphraseReq,
		hostKeyRequest:    cfg.HostKeyRequest,
	}
}

// OpenSession creates a new session for the given connection ID.
// Returns the session ID immediately; the connection happens asynchronously.
func (m *SessionManager) OpenSession(ctx context.Context, connectionID string) (string, error) {
	conn, err := m.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}

	if err := conn.ValidateForConnect(); err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}

	proto := conn.GetProtocol()
	sessionID := generateSessionID()
	sessionCtx, cancel := context.WithCancel(context.Background())

	entry := &sessionEntry{
		info: domain.ConnectionSession{
			SessionID:      sessionID,
			ConnectionID:   connectionID,
			ConnectionName: conn.Name,
			Protocol:       proto,
			State:          domain.SessionConnecting,
		},
		ctx:          sessionCtx,
		cancel:       cancel,
		connectionID: connectionID,
	}

	m.mu.Lock()
	m.sessions[sessionID] = entry
	m.mu.Unlock()

	m.notifyStateChange(entry.info)

	if proto == domain.ProtocolSSH {
		go m.connectSession(entry, conn)
	} else {
		connector := m.connectors[proto]
		if connector == nil {
			m.updateState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", proto))
			return sessionID, nil
		}
		go m.runConnector(entry, conn, connector)
	}

	return sessionID, nil
}

// CloseSession terminates a session by its ID, releasing all resources.
func (m *SessionManager) CloseSession(sessionID string) error {
	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domain.ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	entry.cancel()

	if entry.ptyBridge != nil {
		if err := entry.ptyBridge.Close(); err != nil {
			slog.Warn("close pty bridge failed", "sessionID", sessionID, "err", err)
		}
	}
	if entry.remoteFS != nil {
		if err := entry.remoteFS.Close(); err != nil {
			slog.Warn("close remote fs failed", "sessionID", sessionID, "err", err)
		}
	}
	if entry.sshClient != nil {
		if err := entry.sshClient.Close(); err != nil {
			slog.Warn("close ssh client failed", "sessionID", sessionID, "err", err)
		}
	}

	entry.info.State = domain.SessionClosed
	m.notifyStateChange(entry.info)
	return nil
}

// CloseAll terminates all active sessions. Used during application shutdown.
func (m *SessionManager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.CloseSession(id)
	}
	m.passphraseCache.Clear()
}

// GetState returns the current session info for a given session ID.
func (m *SessionManager) GetState(sessionID string) (domain.ConnectionSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return domain.ConnectionSession{}, domain.ErrSessionNotFound
	}
	return entry.info, nil
}

// GetAllSessions returns info for all active sessions.
func (m *SessionManager) GetAllSessions() []domain.ConnectionSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]domain.ConnectionSession, 0, len(m.sessions))
	for _, entry := range m.sessions {
		result = append(result, entry.info)
	}
	return result
}

// runConnector invokes a non-SSH connector with the appropriate hooks.
func (m *SessionManager) runConnector(entry *sessionEntry, conn *domain.Connection, connector domain.SessionConnector) {
	deps := domain.ConnectorDeps{
		ConnRepo:     m.connRepo,
		VaultRepo:    m.vaultRepo,
		IdentRepo:    m.identRepo,
		PasswordRepo: m.passwordRepo,
		KnownHosts:   m.knownHosts,
		SSHFactory:   m.sshFactory,
	}
	hooks := domain.ConnectorHooks{
		SetSSHClient: func(c domain.SSHClient) {
			m.mu.Lock()
			if e, ok := m.sessions[entry.info.SessionID]; ok {
				e.sshClient = c
			}
			m.mu.Unlock()
		},
		SetRemoteFS: func(fs domain.RemoteFS) {
			m.mu.Lock()
			if e, ok := m.sessions[entry.info.SessionID]; ok {
				e.remoteFS = fs
			}
			m.mu.Unlock()
		},
		SetPTYBridge: func(b domain.TerminalPTYBridge) {
			m.mu.Lock()
			if e, ok := m.sessions[entry.info.SessionID]; ok {
				e.ptyBridge = b
			}
			m.mu.Unlock()
		},
		UpdateState: func(s domain.SessionState, msg string) {
			m.updateState(entry, s, msg)
		},
		SetHostKeyInfo: func(h *domain.HostKeyInfo) {
			m.mu.Lock()
			if e, ok := m.sessions[entry.info.SessionID]; ok {
				e.hostKeyInfo = h
			}
			m.mu.Unlock()
		},
		OnStreamReady: func(ch <-chan []byte) {
			if m.onStreamReady != nil {
				m.onStreamReady(entry.info.SessionID, ch)
			}
		},
	}
	if err := connector.Connect(entry.ctx, conn, deps, hooks); err != nil {
		slog.Debug("connector finished with error", "sessionID", entry.info.SessionID, "err", err)
	}
}
