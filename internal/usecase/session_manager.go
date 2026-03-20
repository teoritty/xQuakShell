package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
	infrassh "ssh-client/internal/infra/ssh"
)

const serverAliveInterval = 30 * time.Second

// sessionEntry holds runtime state for a single session (tab).
type sessionEntry struct {
	info         domain.ConnectionSession
	ctx          context.Context
	cancel       context.CancelFunc
	sshClient    domain.SSHClient
	remoteFS     domain.RemoteFS
	ptyBridge    domain.TerminalPTYBridge
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

// SessionManager manages the lifecycle of parallel sessions (tabs) for all protocols.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry

	connRepo        domain.ConnectionRepository
	vaultRepo       domain.VaultRepository
	identRepo       domain.IdentityRepository
	passwordRepo    domain.PasswordRepository
	knownHosts      domain.KnownHostsRepository
	sshFactory      domain.SSHClientFactory
	passphraseCache *infrassh.PassphraseCache
	connectors      map[string]domain.SessionConnector

	onStateChange   StateChangeFunc
	onStreamReady   OnStreamReadyFunc
	passphraseReq   PassphraseRequestFunc
	hostKeyRequest  HostKeyRequestFunc
}

// OnStreamReadyFunc is called when a stream connector (Telnet, Serial) has started
// the terminal output. The API uses this to begin streaming to the frontend.
type OnStreamReadyFunc func(sessionID string, outputCh <-chan []byte)

// SessionManagerConfig holds dependencies for creating a SessionManager.
type SessionManagerConfig struct {
	ConnRepo       domain.ConnectionRepository
	VaultRepo      domain.VaultRepository
	IdentRepo      domain.IdentityRepository
	PasswordRepo   domain.PasswordRepository
	KnownHosts     domain.KnownHostsRepository
	SSHFactory     domain.SSHClientFactory
	Connectors     []domain.SessionConnector
	OnStateChange  StateChangeFunc
	OnStreamReady  OnStreamReadyFunc
	PassphraseReq  PassphraseRequestFunc
	HostKeyRequest HostKeyRequestFunc
}

// NewSessionManager creates a SessionManager with the given dependencies.
func NewSessionManager(cfg SessionManagerConfig) *SessionManager {
	connectors := make(map[string]domain.SessionConnector)
	for _, c := range cfg.Connectors {
		connectors[c.Protocol()] = c
	}
	return &SessionManager{
		sessions:        make(map[string]*sessionEntry),
		connRepo:        cfg.ConnRepo,
		vaultRepo:       cfg.VaultRepo,
		identRepo:       cfg.IdentRepo,
		passwordRepo:    cfg.PasswordRepo,
		knownHosts:      cfg.KnownHosts,
		sshFactory:      cfg.SSHFactory,
		passphraseCache: infrassh.NewPassphraseCache(),
		connectors:      connectors,
		onStateChange:   cfg.OnStateChange,
		onStreamReady:   cfg.OnStreamReady,
		passphraseReq:   cfg.PassphraseReq,
		hostKeyRequest:  cfg.HostKeyRequest,
	}
}

// OpenSession creates a new session for the given connection ID.
// Returns the session ID immediately; the connection happens asynchronously.
func (m *SessionManager) OpenSession(connectionID string) (string, error) {
	conn, err := m.connRepo.GetByID(context.Background(), connectionID)
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}

	if err := conn.ValidateForConnect(); err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}

	proto := conn.GetProtocol()
	sessionID := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())

	entry := &sessionEntry{
		info: domain.ConnectionSession{
			SessionID:      sessionID,
			ConnectionID:   connectionID,
			ConnectionName: conn.Name,
			Protocol:       proto,
			State:          domain.SessionConnecting,
		},
		ctx:          ctx,
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
	_ = connector.Connect(entry.ctx, conn, deps, hooks)
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
		entry.ptyBridge.Close()
	}
	if entry.remoteFS != nil {
		entry.remoteFS.Close()
	}
	if entry.sshClient != nil {
		entry.sshClient.Close()
	}

	entry.info.State = domain.SessionClosed
	m.notifyStateChange(entry.info)
	return nil
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

// SetSessionEmbedError marks the session as failed (e.g. embedded RDP connect error on Windows).
func (m *SessionManager) SetSessionEmbedError(sessionID string, errMsg string) error {
	m.mu.RLock()
	entry, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return domain.ErrSessionNotFound
	}
	m.updateState(entry, domain.SessionError, errMsg)
	return nil
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

// SetPTYBridge stores the PTY bridge for a session.
func (m *SessionManager) SetPTYBridge(sessionID string, bridge domain.TerminalPTYBridge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[sessionID]; ok {
		entry.ptyBridge = bridge
	}
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

// connectSession performs the SSH handshake in a goroutine.
// It resolves the default user's auth config and, if jump chain is configured,
// establishes intermediate bastion connections first.
// On host key errors it transitions to SessionHostKeyRequired and waits for RetrySession.
func (m *SessionManager) connectSession(entry *sessionEntry, conn *domain.Connection) {
	signers, password, err := m.resolveAuth(entry.ctx, conn)
	if err != nil {
		m.updateState(entry, domain.SessionError, fmt.Sprintf("auth loading: %s", err.Error()))
		return
	}

	hostChecker := infrassh.NewHostKeyChecker(m.knownHosts)
	hostKeyCallback := hostChecker.HostKeyCallback()

	timeoutSec := 15
	if data, err := m.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.ConnectionTimeoutSec > 0 {
		timeoutSec = data.Settings.Transfer.ConnectionTimeoutSec
	}

	sshCfg := domain.SSHClientConfig{
		Host:            conn.Host,
		Port:            conn.Port,
		User:            conn.EffectiveUsername(),
		Signers:         signers,
		Password:        password,
		HostKeyCallback: hostKeyCallback,
		TimeoutSeconds:  timeoutSec,
	}
	if conn.Proxy != nil && !conn.Proxy.IsEmpty() {
		proxyAuth := &domain.ProxyAuth{
			Host:     conn.Proxy.Host,
			Port:     conn.Proxy.Port,
			Username: conn.Proxy.Username,
		}
		if conn.Proxy.PasswordID != "" {
			pw, err := m.passwordRepo.Get(entry.ctx, conn.Proxy.PasswordID)
			if err == nil {
				proxyAuth.Password = string(pw)
			}
		}
		sshCfg.Proxy = proxyAuth
	}

	if !conn.JumpChain.IsEmpty() {
		var proxyAuth *domain.ProxyAuth
		if conn.Proxy != nil && !conn.Proxy.IsEmpty() {
			proxyAuth = &domain.ProxyAuth{Host: conn.Proxy.Host, Port: conn.Proxy.Port, Username: conn.Proxy.Username}
			if conn.Proxy.PasswordID != "" {
				if pw, err := m.passwordRepo.Get(entry.ctx, conn.Proxy.PasswordID); err == nil {
					proxyAuth.Password = string(pw)
				}
			}
		}
		transport, chainCleanup, chainErr := infrassh.BuildTransportChain(
			entry.ctx,
			conn.JumpChain.Hops,
			conn.Host, conn.Port,
			timeoutSec,
			proxyAuth,
			m.sshFactory,
			hostKeyCallback,
			m.resolveHopAuth,
		)
		if chainErr != nil {
			if errors.Is(chainErr, domain.ErrUnknownHost) || errors.Is(chainErr, domain.ErrHostKeyMismatch) {
				m.handleHostKeyError(entry, conn, chainErr)
				return
			}
			m.updateState(entry, domain.SessionError, fmt.Sprintf("jump chain: %s", chainErr.Error()))
			return
		}
		sshCfg.Transport = transport

		go func() {
			<-entry.ctx.Done()
			chainCleanup()
		}()
	}

	client, err := m.sshFactory.Create(entry.ctx, sshCfg)
	if err != nil {
		if errors.Is(err, domain.ErrUnknownHost) || errors.Is(err, domain.ErrHostKeyMismatch) {
			m.handleHostKeyError(entry, conn, err)
			return
		}
		m.updateState(entry, domain.SessionError, err.Error())
		return
	}

	m.mu.Lock()
	entry.sshClient = client
	m.mu.Unlock()

	go m.runServerAlive(entry)

	m.updateState(entry, domain.SessionReady, "")
}

// runServerAlive sends periodic keepalive requests to detect connection loss.
func (m *SessionManager) runServerAlive(entry *sessionEntry) {
	ticker := time.NewTicker(serverAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-entry.ctx.Done():
			return
		case <-ticker.C:
			_, _, err := entry.sshClient.Client().SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				m.NotifySessionDisconnected(entry.info.SessionID)
				return
			}
		}
	}
}

func (m *SessionManager) handleHostKeyError(entry *sessionEntry, conn *domain.Connection, err error) {
	mismatch := errors.Is(err, domain.ErrHostKeyMismatch)

	hkInfo := domain.HostKeyInfo{
		Host:     fmt.Sprintf("%s:%d", conn.Host, conn.Port),
		Mismatch: mismatch,
	}

	var hkErr *infrassh.HostKeyError
	if errors.As(err, &hkErr) && hkErr.Key != nil {
		extracted := infrassh.ExtractHostKeyInfo(hkErr.Host, hkErr.Key)
		hkInfo.Host = extracted.Host
		hkInfo.KeyType = extracted.KeyType
		hkInfo.Fingerprint = extracted.Fingerprint
		hkInfo.KeyBase64 = extracted.KeyBase64
	}

	m.mu.Lock()
	entry.hostKeyInfo = &hkInfo
	m.mu.Unlock()
	m.updateState(entry, domain.SessionHostKeyRequired, err.Error())
	if m.hostKeyRequest != nil {
		m.hostKeyRequest(entry.info.SessionID, hkInfo)
	}
}

// resolveHopAuth resolves auth credentials for a single jump hop.
func (m *SessionManager) resolveHopAuth(hop domain.JumpHop) ([]gossh.Signer, string, error) {
	switch hop.Auth {
	case domain.AuthMethodKey:
		if hop.KeyAuth == nil {
			return nil, "", nil
		}
		signers, err := m.loadSignersLegacy(context.Background(), hop.KeyAuth.IdentityIDs)
		return signers, "", err
	case domain.AuthMethodPassword:
		if hop.PassAuth == nil || hop.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("hop password auth but no password ID")
		}
		pw, err := m.passwordRepo.Get(context.Background(), hop.PassAuth.PasswordID)
		if err != nil {
			return nil, "", err
		}
		return nil, string(pw), nil
	default:
		return nil, "", nil
	}
}

// RetrySession re-attempts the SSH connection for a session in hostkey-required state.
// Called after the user has added/replaced the host key via the UI.
func (m *SessionManager) RetrySession(sessionID string) error {
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

	conn, err := m.connRepo.GetByID(context.Background(), connID)
	if err != nil {
		m.updateState(entry, domain.SessionError, err.Error())
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

// resolveAuth determines SSH auth (signers and/or password) from the connection's
// default user. Falls back to legacy IdentityIDs for v1 connections.
func (m *SessionManager) resolveAuth(ctx context.Context, conn *domain.Connection) ([]gossh.Signer, string, error) {
	defaultUser := conn.DefaultUser()
	if defaultUser == nil {
		signers, err := m.loadSignersLegacy(ctx, conn.IdentityIDs)
		return signers, "", err
	}

	switch defaultUser.Auth {
	case domain.AuthMethodKey:
		if defaultUser.KeyAuth == nil {
			return nil, "", nil
		}
		signers, err := m.loadSignersLegacy(ctx, defaultUser.KeyAuth.IdentityIDs)
		return signers, "", err

	case domain.AuthMethodPassword:
		if defaultUser.PassAuth == nil || defaultUser.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("password auth configured but no password ID set")
		}
		passwordBytes, err := m.passwordRepo.Get(ctx, defaultUser.PassAuth.PasswordID)
		if err != nil {
			return nil, "", fmt.Errorf("load password: %w", err)
		}
		return nil, string(passwordBytes), nil

	default:
		return nil, "", fmt.Errorf("unknown auth method %q", defaultUser.Auth)
	}
}

// loadSignersLegacy reads private keys by their IDs and parses them into SSH signers.
func (m *SessionManager) loadSignersLegacy(ctx context.Context, identityIDs []string) ([]gossh.Signer, error) {
	if len(identityIDs) == 0 {
		return nil, nil
	}

	signers := make([]gossh.Signer, 0, len(identityIDs))
	for _, idRef := range identityIDs {
		pemData, err := m.identRepo.GetKeyBlob(ctx, idRef)
		if err != nil {
			return nil, fmt.Errorf("load key %s: %w", idRef, err)
		}

		passphrase, _ := m.passphraseCache.Get(idRef)
		signer, err := infrassh.ParseKeyWithPassphrase(pemData, passphrase)
		if err != nil {
			if err == domain.ErrPassphraseRequired && m.passphraseReq != nil {
				identMeta, _ := m.getIdentityMeta(ctx, idRef)
				comment := idRef
				if identMeta != nil {
					comment = identMeta.Comment
				}
				pp, ppErr := m.passphraseReq(idRef, comment)
				if ppErr != nil {
					return nil, fmt.Errorf("passphrase request for %s: %w", idRef, ppErr)
				}
				signer, err = infrassh.ParseKeyWithPassphrase(pemData, pp)
				if err != nil {
					return nil, fmt.Errorf("parse key %s with passphrase: %w", idRef, err)
				}
				m.passphraseCache.Set(idRef, pp)
			} else {
				return nil, fmt.Errorf("parse key %s: %w", idRef, err)
			}
		}

		signers = append(signers, signer)
	}
	return signers, nil
}

func (m *SessionManager) getIdentityMeta(ctx context.Context, id string) (*domain.SSHIdentity, error) {
	data, err := m.vaultRepo.GetData()
	if err != nil {
		return nil, err
	}
	ident, ok := data.Identities[id]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return &ident, nil
}

func (m *SessionManager) updateState(entry *sessionEntry, state domain.SessionState, errMsg string) {
	m.mu.Lock()
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
