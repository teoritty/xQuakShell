package plugin

import (
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

const (
	initTimeout         = 10 * time.Second
	callTimeout         = 5 * time.Second
	shutdownCallTimeout = 2 * time.Second
	stopGracePeriod     = 3 * time.Second
)

// ProcessCrashHandler is notified when a plugin process exits abnormally.
type ProcessCrashHandler func(pluginID, sessionID string)

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
	ChannelAudit    domainplugin.ChannelAuditRecorder
	ChannelBus      *capability.ChannelBus
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
