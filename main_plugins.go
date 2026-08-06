package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/auditlog"
	infraplugin "xquakshell/internal/infra/plugin"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	"xquakshell/internal/infra/plugin/capability"
	infrapluginlifecycle "xquakshell/internal/infra/plugin/lifecycle"
	infraportable "xquakshell/internal/infra/portable"
	"xquakshell/internal/pkg/ratelimit"
	"xquakshell/internal/pkg/safego"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

type pluginRuntime struct {
	inbound             *usecase.PluginSessionInbound
	embedInbound        *usecase.PluginEmbedInbound
	embedTunnels        *usecase.EmbedTunnelService
	dynamicForward      *usecase.DynamicForwardCoordinator
	embedBridge         *usecase.PluginEmbedBridge
	channelBus          *capability.ChannelBus
	sessionRegistry     *sessionRegistryHolder
	viewInbound         *usecase.PluginViewInbound
	discoveryService    *usecase.DiscoveryService
	discoveryLeader     *usecase.DiscoveryLeader
	surfaces            *usecase.SurfaceService
	dialogs             *usecase.DialogService
	nodeDetails         *usecase.DiscoveryDetailsService
	discoveryEmit       *discoveryEmitHolder
	viewRelay           *usecase.PluginViewRelay
	vaultInbound        *usecase.PluginVaultInbound
	vaultSettings       *usecase.PluginVaultSettings
	manager             *usecase.PluginManager
	supervisor          *usecase.PluginSupervisor
	githubRepoService   *usecase.GitHubRepositoryService
	githubPluginService *usecase.GitHubPluginService
	connRepo            domain.ConnectionRepository
	host                *infraplugin.ProcessHost
	assets              http.Handler
	cancel              context.CancelFunc
}

type pluginRuntimeDeps struct {
	ConnRepo        domain.ConnectionRepository
	PasswordRepo    domain.PasswordRepository
	IdentRepo       domain.IdentityRepository
	AuditLog        domain.AuditLogRepository
	VaultSettings   *usecase.PluginVaultSettings
	PassphraseCache domain.PassphraseCache
	ExeDir          string
}

func newPluginRuntime(dataRoot string, portableData domain.PortableDataStore, deps pluginRuntimeDeps) *pluginRuntime {
	inbound := usecase.NewPluginSessionInbound()
	embedInbound := usecase.NewPluginEmbedInbound()
	embedTunnels := usecase.NewEmbedTunnelService(ratelimit.Factory{})
	registry := usecase.NewPluginRegistry()
	// Discovery icons are read from the installed bundle as each plugin enters the registry and
	// travel to the frontend as data URIs on the plugin list (ADR-014): no icon endpoint, no path
	// ever leaving the backend. Must be set before DiscoverPlugins below.
	registry.SetDiscoveryIconAssetReader(infrapluginassets.DiscoveryIconReader{})
	viewInbound := usecase.NewPluginViewInbound(registry)
	portableRuntime := infraportable.NewRuntimeAdapter()

	vaultInbound := usecase.NewPluginVaultInbound(
		registry,
		deps.ConnRepo,
		deps.PasswordRepo,
		deps.IdentRepo,
		deps.VaultSettings,
		deps.PassphraseCache,
	)
	vaultAudit, err := auditlog.NewNDJSONVaultAuditLogger(dataRoot)
	if err != nil {
		log.Printf("WARNING: plugin vault audit logger init failed: %v", err)
	} else {
		vaultInbound.SetAuditLogger(vaultAudit)
	}

	pluginAudit := usecase.NewPluginAuditWriter(deps.AuditLog)

	sessionAuthorizer := usecase.NewPluginSessionAuthorizer(registry)
	sessionAuthorizer.SetSettingsReader(deps.VaultSettings)
	sessionAuthorizer.SetBindAudit(pluginAudit.SessionBindFunc())

	// The manager does not exist until the host it drives does, and the host's config closes over
	// it. These two references are that cycle, written down once: everything that needs the manager
	// before it exists reads them, and they are filled the moment it does.
	var managerRef *usecase.PluginManager
	var supervisorRef *usecase.PluginSupervisor
	dynamicForward := usecase.NewDynamicForwardCoordinator(nil, nil)
	eventBus := usecase.NewPluginEventBus(registry, func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
		if managerRef == nil {
			return nil
		}
		return managerRef.NotifyProcess(ctx, pluginID, sessionID, method, params)
	})

	channelBus := capability.NewChannelBus()
	sessionRegistry := newSessionRegistryHolder()
	// The discovery service cannot exist yet: it needs the PluginManager, which needs the host,
	// which needs this session RPC factory. The holder closes that cycle the same way
	// sessionRegistryHolder does — the port is resolved at CALL time, and a plugin process cannot
	// call discovery.publish before it has been started by the very manager that fills it in.
	discoveryInbound := newDiscoveryInboundHolder()
	surfaceInbound := newSurfaceInboundHolder()
	dialogInbound := newDialogInboundHolder()
	detailsInbound := newDetailsInboundHolder()

	// The process host, the manager that drives it and the supervisor that restarts it. Assembled
	// in main_plugin_host.go: what a plugin process is allowed to reach is one subject, and it is
	// the subject with the most moving parts.
	host, manager, supervisor := buildPluginHost(pluginHostDeps{
		DataRoot:        dataRoot,
		PortableData:    portableData,
		PortableRuntime: portableRuntime,
		Registry:        registry,
		Settings:        deps.VaultSettings,
		ConnRepo:        deps.ConnRepo,
		Audit:           pluginAudit,
		Authorizer:      sessionAuthorizer,
		Vault:           vaultInbound,
		Views:           viewInbound,
		Events:          eventBus,
		SessionRPC: sessionRPCPorts{
			Sessions:  inbound,
			Embed:     embedInbound,
			Discovery: discoveryInbound,
			Surfaces:  surfaceInbound,
			Dialogs:   dialogInbound,
			Details:   detailsInbound,
		},
		ChannelBus:      channelBus,
		SessionRegistry: sessionRegistry,
		EmbedTunnels:    embedTunnels,
		Tunnel:          dynamicForward,
		ManagerRef:      &managerRef,
		SupervisorRef:   &supervisorRef,
	})
	managerRef, supervisorRef = manager, supervisor
	viewRelay := usecase.NewPluginViewRelay(manager, registry)

	eventBus.SetSessionActiveChecker(func(pluginID string) bool {
		return manager.ActiveSessionCount(pluginID) > 0
	})

	// Discovery subtrees (ADR-014) and the ui services built on them (ADR-015) are assembled next
	// door, in main_plugin_discovery.go and main_plugin_ui.go.
	discovery := buildDiscoveryStack(discoveryStackDeps{
		Manager:  manager,
		Registry: registry,
		Sessions: sessionRegistry,
		Audit:    pluginAudit,
	}, discoveryInbound)

	// The ADR-015 services, and the process-lifecycle answers they share with discovery, are
	// assembled next door in main_plugin_ui.go.
	ui := buildUIStack(uiStackDeps{
		Manager:  manager,
		Registry: registry,
		Sessions: sessionRegistry,
		Audit:    pluginAudit,
		Store:    discovery.store,
		Leader:   discovery.leader,
		Pace:     discovery.pace,
	}, surfaceInbound, dialogInbound, detailsInbound)
	wirePluginProcessLifecycle(manager, supervisor, discovery.observer, discovery.service, discovery.leader, ui)

	pluginDiscovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(deps.ExeDir, dataRoot))
	if err := manager.DiscoverPlugins(pluginDiscovery.Discover); err != nil {
		log.Printf("WARNING: plugin discovery failed: %v", err)
	}

	github := buildGitHubServices(dataRoot, portableData, manager)

	ctx, cancel := context.WithCancel(context.Background())
	safego.GoNamed("plugin.idleSuspender", func() {
		infrapluginlifecycle.RunIdleSuspender(ctx, manager, infrapluginlifecycle.Config{
			IdleAfter: 5 * time.Minute,
			TickEvery: time.Minute,
		})
	})

	// Plugin UI assets and the embed broker, including the loopback listener the broker needs on
	// Windows. See main_plugin_assets.go for why it cannot simply ride the Wails asset server.
	compositeAssets := buildPluginAssetHandler(registry, embedTunnels)

	embedTunnels.SetPluginNotifier(func(ctx context.Context, pluginID, sessionID, method string, params []byte) error {
		if manager == nil {
			return nil
		}
		return manager.NotifyForSession(ctx, pluginID, sessionID, method, json.RawMessage(params))
	})
	dynamicForward.SetNotifier(func(ctx context.Context, pluginID, sessionID, method string, params []byte) error {
		if manager == nil {
			return nil
		}
		return manager.Notify(ctx, pluginID, method, json.RawMessage(params))
	})
	dynamicForward.SetStarter(manager)

	return &pluginRuntime{
		inbound:             inbound,
		embedInbound:        embedInbound,
		embedTunnels:        embedTunnels,
		channelBus:          channelBus,
		sessionRegistry:     sessionRegistry,
		dynamicForward:      dynamicForward,
		discoveryService:    discovery.service,
		discoveryLeader:     discovery.leader,
		surfaces:            ui.surfaces,
		dialogs:             ui.dialogs,
		nodeDetails:         ui.nodeDetails,
		discoveryEmit:       discovery.emit,
		viewInbound:         viewInbound,
		viewRelay:           viewRelay,
		vaultInbound:        vaultInbound,
		vaultSettings:       deps.VaultSettings,
		manager:             manager,
		supervisor:          supervisor,
		githubRepoService:   github.repos,
		githubPluginService: github.plugins,
		connRepo:            deps.ConnRepo,
		host:                host,
		assets:              compositeAssets,
		cancel:              cancel,
	}
}

func (r *pluginRuntime) wireEmbed(api *presentation.AppAPI) {
	if r == nil || api == nil {
		return
	}
	r.embedBridge = usecase.NewPluginEmbedBridge(r.manager, r.embedTunnels, r.embedTunnels)
	if r.embedInbound != nil {
		r.embedInbound.SetHandler(r.embedTunnels)
	}
	if r.embedTunnels != nil {
		r.embedTunnels.SetEmbedReadyHandler(api.OnEmbedReady)
	}
	api.Sessions().SetEmbedTunnelService(r.embedTunnels)
	// The exec channel backend needs the session registry NewSessionManager owns privately, and
	// this runtime -- resolver included -- was built before it existed. SessionManager pushes it
	// here rather than exposing it, the same way it hands it to EmbedTunnelService above.
	api.Sessions().WireChannelSessionRegistry(r.sessionRegistry.set)
	api.Sessions().SetChannelBus(r.channelBus)
	api.Sessions().SetDiscovery(r.discoveryLeader)
	if r.discoveryService != nil {
		api.SetDiscoveryService(r.discoveryService)
	}
	if r.discoveryEmit != nil {
		r.discoveryEmit.set(api.EmitDiscoveryTreeChanged)
	}
	r.wireUIPresenters(api)
	api.Sessions().SetDynamicForward(r.dynamicForward)
	if r.dynamicForward != nil && r.vaultSettings != nil {
		r.dynamicForward.SetTunnelGrantReader(r.vaultSettings)
	}
	if r.dynamicForward != nil && r.host != nil {
		r.dynamicForward.SetDialSlotReleaser(r.host.ReleaseTunnelDialSlot)
	}
	if r.manager != nil && r.vaultSettings != nil && r.connRepo != nil {
		validator := usecase.NewForwardRuleValidator(r.connRepo, r.manager.Registry(), r.vaultSettings)
		api.SetForwardRuleValidator(validator)
		api.Sessions().SetForwardRuleValidator(validator)
	}
	api.SetEmbedBridge(r.embedBridge)
}

func (r *pluginRuntime) shutdown() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *pluginRuntime) assetHandler() http.Handler {
	if r == nil {
		return nil
	}
	return r.assets
}

func (r *pluginRuntime) grantMultiSessionAccess(ctx context.Context, pluginID string) error {
	if r == nil || r.vaultSettings == nil {
		return nil
	}
	return r.vaultSettings.GrantMultiSessionAccess(ctx, pluginID)
}

func (r *pluginRuntime) grantSecretAccess(ctx context.Context, pluginID string) error {
	if r == nil || r.vaultSettings == nil {
		return nil
	}
	return r.vaultSettings.GrantSecretAccess(ctx, pluginID)
}

func (r *pluginRuntime) grantAuthProviderAccess(ctx context.Context, pluginID string) error {
	if r == nil || r.vaultSettings == nil {
		return nil
	}
	return r.vaultSettings.GrantAuthProviderAccess(ctx, pluginID)
}

func (r *pluginRuntime) grantTunnelProviderAccess(ctx context.Context, pluginID string) error {
	if r == nil || r.vaultSettings == nil {
		return nil
	}
	return r.vaultSettings.GrantTunnelProviderAccess(ctx, pluginID)
}

func (r *pluginRuntime) grantArbitraryNetworkAccess(ctx context.Context, pluginID string) error {
	if r == nil || r.vaultSettings == nil {
		return nil
	}
	return r.vaultSettings.GrantArbitraryNetworkAccess(ctx, pluginID)
}

func (r *pluginRuntime) setSessionRecoverer(recoverer usecase.PluginSessionRecoverer) {
	if r == nil || r.supervisor == nil {
		return
	}
	r.supervisor.SetRecoverer(recoverer)
}
