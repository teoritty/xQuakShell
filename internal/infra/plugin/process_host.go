package plugin

import (
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/domain"
	"ssh-client/internal/infra/plugin/ipc"
)

const (
	coreAPIVersion      = "1.0.0"
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
	SessionAuthorizer domainplugin.SessionRPCAuthorizer
	Audit             ipc.PluginAuditFunc
	OnCrash           ProcessCrashHandler
	OnPluginActivity  func(pluginID string)
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
