package usecase

import (
	"time"

	"xquakshell/internal/domain"
)

// SessionManagerConfig holds dependencies for creating a SessionManager.
type SessionManagerConfig struct {
	ConnRepo                   domain.ConnectionRepository
	VaultRepo                  domain.VaultRepository
	IdentRepo                  domain.IdentityRepository
	PasswordRepo               domain.PasswordRepository
	KnownHosts                 domain.KnownHostsRepository
	SSHFactory                 domain.SSHClientFactory
	PassphraseCache            domain.PassphraseCache
	HostKeyCallbackBuilder     domain.HostKeyCallbackBuilder
	JumpTransportBuilder       domain.JumpTransportBuilder
	PrivateKeySignerFactory    domain.PrivateKeySignerFactory
	PTYBridgeFactory           domain.PTYBridgeFactory
	SFTPClientFactory          domain.SFTPClientFactory
	Connectors                 []domain.SessionConnector
	PluginBridge               *PluginSessionBridge
	OnStateChange              StateChangeFunc
	OnStreamReady              OnStreamReadyFunc
	PassphraseReq              PassphraseRequestFunc
	HostKeyRequest             HostKeyRequestFunc
	PluginTerminalWriteTimeout time.Duration
	AuthProvider               domain.PluginAuthProvider
	AuthMethodBuilder          domain.PluginAuthMethodBuilder
	AuthAttempts               *PluginAuthAttemptRegistry
	AuthLookup                 PluginAuthMethodLookup
	AuthStarter                PluginAuthStarter
	AuthGrantReader            PluginAuthGrantReader
	DynamicForward             *DynamicForwardCoordinator
	ForwardConnLimiterFactory  func() domain.ConcurrencyLimiter
}
