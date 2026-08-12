package plugin

import (
	"log/slog"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
	"xquakshell/internal/infra/plugin/sandbox"
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
	// (ADR-011) can reach it; process exit/crash teardown does not depend on it.
	ChannelResolverFor func(plugin domainplugin.InstalledPlugin, sessionID string) (capability.ChannelBackendResolver, AttachChannelCloseNotify)

	ChannelAudit domainplugin.ChannelAuditRecorder
	ChannelBus   *capability.ChannelBus
}

type ProcessHost struct {
	cfg       HostConfig
	mu        sync.Mutex
	processes map[string]*managedProcess
	// sandbox is what this build can enforce, asked once. It is a property of the build today, not
	// of any one process: no platform confines a plugin yet, so every process gets the same answer
	// and Start has nothing to decide. When a platform can enforce, the outcome becomes per-process
	// — a profile can fail for one plugin and not another — and this moves onto managedProcess,
	// which is why ProcessInstance already carries it per instance rather than per host.
	sandbox domainplugin.SandboxSupport
}

func NewProcessHost(cfg HostConfig) *ProcessHost {
	support := sandbox.Support()
	if !support.Available {
		slog.Info("plugin OS isolation unavailable", "reason", support.Reason)
	}
	// Startup is the one moment when no plugin of ours is running, so it is the only safe time to
	// delete the durable per-instance state a crash or a power loss left behind. Nothing here can
	// fail the construction: this is housekeeping, and a profile that will not delete is not a
	// reason to refuse to start.
	SweepOrphanContainers()
	return &ProcessHost{
		cfg:       cfg,
		processes: make(map[string]*managedProcess),
		sandbox:   support,
	}
}

var _ domainplugin.ProcessHost = (*ProcessHost)(nil)
