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

	// ChannelResolver resolves the backend for a channel.open purpose (exec/tcp-relay/embed-stream
	// land in later stages). ChannelAudit records channel.open/channel.close events. ChannelBus
	// registers each process's ChannelProxy so SessionLifecycleService's CloseSession cascade
	// (ADR-011 Stage 4) can reach it; process exit/crash teardown (Stage 4b) does not depend on it.
	ChannelResolver capability.ChannelBackendResolver
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
