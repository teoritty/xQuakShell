package main

import (
	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	infrapluginbundle "xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/usecase"
)

// The plugin process host, the manager that drives it and the supervisor that restarts it.
//
// This is the boundary a plugin process talks through, so what it is allowed to reach is decided
// once, here, rather than spread through the constructor that happens to build it.

// sessionRPCPorts is every inbound port a plugin can address over session RPC. Grouped so the
// factory takes one argument that says what it is, rather than six positional interfaces of the
// same shape that only the compiler could tell apart — and it could not.
type sessionRPCPorts struct {
	Sessions  domainplugin.SessionInboundPort
	Embed     *usecase.PluginEmbedInbound
	Discovery domainplugin.DiscoveryInboundPort
	Surfaces  domainplugin.SurfaceInboundPort
	Dialogs   domainplugin.DialogInboundPort
	Details   domainplugin.DiscoveryDetailsInboundPort
}

func (p sessionRPCPorts) factory(auth domainplugin.SessionRPCAuthorizer) domainplugin.SessionRPCHandlerFactory {
	return usecase.NewPluginSessionRPCHandlerFactory(
		p.Sessions, p.Embed, p.Discovery, p.Surfaces, p.Dialogs, p.Details, auth,
	)
}

// pluginHostDeps is what the host and its manager need from the rest of the composition.
type pluginHostDeps struct {
	DataRoot        string
	PortableData    domain.PortableDataStore
	PortableRuntime domain.PortableRuntime
	Registry        *usecase.PluginRegistry
	Settings        *usecase.PluginVaultSettings
	ConnRepo        domain.ConnectionRepository
	Audit           *usecase.PluginAuditWriter
	Authorizer      domainplugin.SessionRPCAuthorizer
	Vault           domainplugin.VaultInboundPort
	Views           *usecase.PluginViewInbound
	Events          *usecase.PluginEventBus
	SessionRPC      sessionRPCPorts
	ChannelBus      *capability.ChannelBus
	SessionRegistry *sessionRegistryHolder
	EmbedTunnels    *usecase.EmbedTunnelService
	Tunnel          domainplugin.TunnelInboundPort

	// ManagerRef and SupervisorRef close the cycle the host config cannot avoid: OnCrash and
	// OnPluginActivity fire from a running process and must reach the manager that started it,
	// which does not exist until this function returns. Pointers to the caller's variables, filled
	// the moment it does.
	ManagerRef    **usecase.PluginManager
	SupervisorRef **usecase.PluginSupervisor
}

func buildPluginHost(deps pluginHostDeps) (*infraplugin.ProcessHost, *usecase.PluginManager, *usecase.PluginSupervisor) {
	hostCfg := infraplugin.HostConfig{
		DataRoot:          deps.DataRoot,
		Portable:          deps.PortableRuntime,
		Vault:             deps.Vault,
		SessionRPC:        deps.SessionRPC.factory(deps.Authorizer),
		Events:            deps.Events,
		Views:             deps.Views,
		Tunnel:            deps.Tunnel,
		SessionAuthorizer: deps.Authorizer,
		Audit:             deps.Audit.RPCRecorder(),
		// The channel purpose backends and everything they need are assembled in
		// main_channels.go: constructing them is its own reason to change, separate from wiring
		// the plugin runtime.
		ChannelResolverFor: newChannelResolverFor(deps.Audit.ChannelFunc(), deps.EmbedTunnels, deps.SessionRegistry),
		ChannelAudit:       deps.Audit.ChannelFunc(),
		ChannelBus:         deps.ChannelBus,
		OnCrash: func(pluginID, sessionID string) {
			if m := *deps.ManagerRef; m != nil {
				m.OnProcessCrashed(pluginID, sessionID)
				if s := *deps.SupervisorRef; s != nil {
					s.HandleCrash(pluginID, sessionID)
				}
			}
		},
		OnPluginActivity: func(pluginID string) {
			if m := *deps.ManagerRef; m != nil {
				m.TouchActivity(pluginID)
			}
		},
	}
	host := infraplugin.NewProcessHost(hostCfg)
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       deps.Registry,
		Host:           host,
		LoadBundle:     infraplugin.LoadPluginSource,
		InstallBundle:  infraplugin.InstallFromSource,
		InstallRoot:    deps.DataRoot,
		PortableData:   deps.PortableData,
		Bundle:         infrapluginbundle.BundleAdapter{},
		Portable:       deps.PortableRuntime,
		PluginSettings: deps.Settings,
		StartAudit:     deps.Audit.StartFunc(),
	})
	manager.SetOutboundAuthAudit(deps.Audit.OutboundAuthFunc())
	manager.SetEventBus(deps.Events)
	manager.SetPluginSettings(deps.Settings)
	manager.SetConnectionChecker(deps.ConnRepo)
	supervisor := usecase.NewPluginSupervisor(manager)

	return host, manager, supervisor
}
