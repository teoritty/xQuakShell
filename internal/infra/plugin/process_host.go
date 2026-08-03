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

// AttachChannelCloseNotify receives one plugin process's channel.close notifier once that process's
// Conn exists. It is returned by ChannelResolverFor, alongside the resolver it belongs to.
type AttachChannelCloseNotify func(notify ChannelCloseNotify)

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
	// It returns a second value for the one thing the resolver cannot be given at build time: this
	// process's channel.close notifier, which must close over a Conn that does not exist yet
	// (the Conn needs the request handler, which needs the proxy the resolver serves). The host
	// calls the returned attach once the Conn is up. Handing it back through the SAME call that
	// built the resolver is what keeps the pairing structural: the two halves meet inside one
	// factory invocation, so a process's notifier cannot be delivered to another process's
	// backends — which a registry keyed by anything reusable, such as the process key, could not
	// promise.
	//
	// ChannelAudit records channel.open/channel.close events. ChannelBus
	// registers each process's ChannelProxy so SessionLifecycleService's CloseSession cascade
	// (ADR-011 Stage 4) can reach it; process exit/crash teardown (Stage 4b) does not depend on it.
	ChannelResolverFor func(plugin domainplugin.InstalledPlugin, sessionID string) (capability.ChannelBackendResolver, AttachChannelCloseNotify)

	ChannelAudit domainplugin.ChannelAuditRecorder
	ChannelBus   *capability.ChannelBus
}

type ProcessHost struct {
	cfg       HostConfig
	mu        sync.Mutex
	processes map[string]*managedProcess
}

func NewProcessHost(cfg HostConfig) *ProcessHost {
	return &ProcessHost{
		cfg:       cfg,
		processes: make(map[string]*managedProcess),
	}
}

var _ domainplugin.ProcessHost = (*ProcessHost)(nil)
