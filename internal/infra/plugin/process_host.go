package plugin

import (
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
)

const (
	initTimeout         = 10 * time.Second
	callTimeout         = 5 * time.Second
	shutdownCallTimeout = 2 * time.Second
	stopGracePeriod     = 3 * time.Second
)

// ProcessCrashHandler is notified when a plugin process exits abnormally.
type ProcessCrashHandler func(pluginID, sessionID string)

// ChannelCloseNotify raises channel.close {channelId, reason, message} on one plugin process's
// connection (ADR-011: application-level errors travel here, not as a binary frame). It is
// structurally identical to usecase.ChannelCloseNotifier, which the composition root converts to:
// infra must not import usecase, so the two halves only meet there.
type ChannelCloseNotify func(channelID uint32, reason, message string)

// HostConfig configures the out-of-process plugin host.
type HostConfig struct {
	DataRoot          string
	Portable          domain.PortableRuntime
	Vault             domainplugin.VaultInboundPort
	SessionRPC        domainplugin.SessionRPCHandlerFactory
	Events            domainplugin.EventInboundPort
	Views             domainplugin.ViewInboundPort
	Tunnel            domainplugin.TunnelInboundPort
	SessionAuthorizer domainplugin.SessionRPCAuthorizer
	Audit             ipc.PluginAuditFunc
	OnCrash           ProcessCrashHandler
	OnPluginActivity  func(pluginID string)

	// ChannelResolverFor builds the resolver serving one plugin process's channel.open calls. It
	// is a factory, not a resolver, because a purpose string alone cannot construct a backend:
	// exec needs the manifest's execCommands and the parent session, the relays need its network
	// caps, embed-stream needs the plugin id. The factory is called once per process, where the
	// manifest is in scope, and the resolver it returns closes over it (same shape as SessionRPC).
	// Only the composition root may supply it — the backends live in usecase, which capability
	// must never import.
	//
	// ChannelAudit records channel.open/channel.close events. ChannelBus
	// registers each process's ChannelProxy so SessionLifecycleService's CloseSession cascade
	// (ADR-011 Stage 4) can reach it; process exit/crash teardown (Stage 4b) does not depend on it.
	ChannelResolverFor func(plugin domainplugin.InstalledPlugin, sessionID string) capability.ChannelBackendResolver

	// AttachChannelCloseNotifier hands the composition root the notifier for one plugin process,
	// once that process's Conn exists. It is an attach rather than a config value for the same
	// reason AttachDataPathOpener is: the notifier must close over Conn.Notify, and the Conn
	// cannot exist before the request handler that routes channel.open, which needs the proxy the
	// resolver serves. The cycle is broken here, after the Conn is built, and only here.
	AttachChannelCloseNotifier func(plugin domainplugin.InstalledPlugin, sessionID string, notify ChannelCloseNotify)

	ChannelAudit domainplugin.ChannelAuditRecorder
	ChannelBus   *capability.ChannelBus
}

// ProcessHost implements domainplugin.ProcessHost using OS child processes.
type ProcessHost struct {
	cfg       HostConfig
	mu        sync.Mutex
	processes map[string]*managedProcess
}

// NewProcessHost creates a process host with capability proxies and audit hooks.
func NewProcessHost(cfg HostConfig) *ProcessHost {
	return &ProcessHost{
		cfg:       cfg,
		processes: make(map[string]*managedProcess),
	}
}

var _ domainplugin.ProcessHost = (*ProcessHost)(nil)
