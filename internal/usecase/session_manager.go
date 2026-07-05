package usecase

import (
	"context"

	"ssh-client/internal/domain"
)

// StateChangeFunc is called whenever a session transitions to a new state.
type StateChangeFunc func(session domain.ConnectionSession)

// PassphraseRequestFunc is called when an encrypted key needs a passphrase.
type PassphraseRequestFunc func(identityID, comment string) (string, error)

// HostKeyRequestFunc is called when a host key decision is needed from the user.
type HostKeyRequestFunc func(sessionID string, info domain.HostKeyInfo)

// OnStreamReadyFunc is called when a stream-based plugin connector has started
// the terminal output. The API uses this to begin streaming to the frontend.
type OnStreamReadyFunc func(sessionID string, outputCh <-chan []byte)

// SessionManager is the composition-root facade Wails/AppAPI code depends on.
//
// WHY THIS IS A THIN FACADE AND NOT WHERE THE LOGIC LIVES (ADR-009):
// Every method here is a one-line delegate. If you're about to add a new
// method with actual logic in this file, don't — put it on
// SessionLifecycleService, SSHConnector, SessionIOService,
// PluginSessionBridge, or EmbedTunnelService instead, and add a delegate
// here only if presentation/wails needs to call it. This file exists so
// external callers don't need to know about the five internal components.
type SessionManager struct {
	lifecycle *SessionLifecycleService
	io        *SessionIOService
	plugins   *PluginSessionBridge
	embed     *EmbedTunnelService
	registry  *SessionRegistry
}

// NewSessionManager creates a SessionManager with the given dependencies.
func NewSessionManager(cfg SessionManagerConfig) *SessionManager {
	registry := NewSessionRegistry()
	sshConnector := NewSSHConnector(SSHConnectorConfig{
		VaultRepo:               cfg.VaultRepo,
		IdentRepo:               cfg.IdentRepo,
		PasswordRepo:            cfg.PasswordRepo,
		KnownHosts:              cfg.KnownHosts,
		SSHFactory:              cfg.SSHFactory,
		PassphraseCache:         cfg.PassphraseCache,
		HostKeyCallbackBuilder:  cfg.HostKeyCallbackBuilder,
		JumpTransportBuilder:    cfg.JumpTransportBuilder,
		PrivateKeySignerFactory: cfg.PrivateKeySignerFactory,
		PassphraseReq:           cfg.PassphraseReq,
	})
	plugins := cfg.PluginBridge
	if plugins == nil {
		plugins = NewPluginSessionBridge(PluginSessionBridgeConfig{})
	}
	lifecycle := NewSessionLifecycleService(SessionLifecycleConfig{
		Registry:        registry,
		ConnRepo:        cfg.ConnRepo,
		SSHConnector:    sshConnector,
		Plugins:         plugins,
		PassphraseCache: cfg.PassphraseCache,
		OnStateChange:   cfg.OnStateChange,
		HostKeyRequest:  cfg.HostKeyRequest,
	})
	io := NewSessionIOService(SessionIOServiceConfig{
		Registry:          registry,
		VaultRepo:         cfg.VaultRepo,
		PTYBridgeFactory:  cfg.PTYBridgeFactory,
		SFTPClientFactory: cfg.SFTPClientFactory,
		OnDisconnected:    lifecycle.NotifySessionDisconnected,
	})
	lifecycle.SetIO(io)
	plugins.WireSessionRuntime(PluginSessionRuntimeConfig{
		Registry:                   registry,
		ConnRepo:                   cfg.ConnRepo,
		OnStateChange:              cfg.OnStateChange,
		OnStreamReady:              cfg.OnStreamReady,
		PluginTerminalWriteTimeout: cfg.PluginTerminalWriteTimeout,
	})
	return &SessionManager{lifecycle: lifecycle, io: io, plugins: plugins, registry: registry}
}

func (m *SessionManager) OpenSession(ctx context.Context, connectionID string) (string, error) {
	return m.lifecycle.OpenSession(ctx, connectionID)
}

func (m *SessionManager) CloseSession(sessionID string) error {
	return m.lifecycle.CloseSession(sessionID)
}

func (m *SessionManager) CloseAll() {
	m.lifecycle.CloseAll()
}

func (m *SessionManager) GetState(sessionID string) (domain.ConnectionSession, error) {
	return m.lifecycle.GetState(sessionID)
}

func (m *SessionManager) GetAllSessions() []domain.ConnectionSession {
	return m.lifecycle.GetAllSessions()
}

func (m *SessionManager) RetrySession(ctx context.Context, sessionID string) error {
	return m.lifecycle.RetrySession(ctx, sessionID)
}

func (m *SessionManager) NotifySessionDisconnected(sessionID string) {
	m.lifecycle.NotifySessionDisconnected(sessionID)
}

func (m *SessionManager) GetHostKeyInfo(sessionID string) (*domain.HostKeyInfo, error) {
	return m.lifecycle.GetHostKeyInfo(sessionID)
}

func (m *SessionManager) PluginBridge() *PluginSessionBridge {
	return m.plugins
}
