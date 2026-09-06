package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/auditlog"
	infraplugin "xquakshell/internal/infra/plugin"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	"xquakshell/internal/infra/plugin/capability"
	infrapluginlifecycle "xquakshell/internal/infra/plugin/lifecycle"
	infraportable "xquakshell/internal/infra/portable"
	"xquakshell/internal/infra/vault"
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
	peerTrustInbound    *peerTrustInboundHolder
	viewRelay           *usecase.PluginViewRelay
	vaultInbound        *usecase.PluginVaultInbound
	vaultSettings       *usecase.PluginVaultSettings
	replicaSync         *usecase.ReplicaSyncService
	manager             *usecase.PluginManager
	supervisor          *usecase.PluginSupervisor
	githubRepoService   *usecase.GitHubRepositoryService
	githubPluginService *usecase.GitHubPluginService
	connRepo            domain.ConnectionRepository
	host                *infraplugin.ProcessHost
	assets              http.Handler
	locales             *usecase.LocaleObserver
	cancel              context.CancelFunc
}

type pluginRuntimeDeps struct {
	ConnRepo        domain.ConnectionRepository
	PasswordRepo    domain.PasswordRepository
	IdentRepo       domain.IdentityRepository
	AuditLog        domain.AuditLogRepository
	VaultSettings   *usecase.PluginVaultSettings
	VaultRepo       domain.VaultRepository
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
	wireVaultInbound(vaultInbound, deps.VaultSettings, dataRoot)

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
	late := newLateBoundPorts()

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
		SessionRPC:      late.sessionRPC(inbound, embedInbound),
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
	}, late.discovery)

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
	}, late.surfaces, late.dialogs, late.details)
	// The interface language, told to the plugins that asked for it. Built here rather than in
	// composeApp because it needs the registry and the manager, and both are assembled in this file.
	locales := usecase.NewLocaleObserver(registry, manager)
	host.SetLocaleSource(locales.Locale)
	wirePluginProcessLifecycle(manager, supervisor, discovery.observer, locales, discovery.service, discovery.leader, ui)

	pluginDiscovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(deps.ExeDir, dataRoot))
	if err := manager.DiscoverPlugins(pluginDiscovery.Discover); err != nil {
		log.Printf("WARNING: plugin discovery failed: %v", err)
	}

	github := buildGitHubServices(dataRoot, portableData, manager)

	ctx, cancel := context.WithCancel(context.Background())
	startIdleSuspender(ctx, manager)

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
		peerTrustInbound:    late.peerTrust,
		viewInbound:         viewInbound,
		viewRelay:           viewRelay,
		vaultInbound:        vaultInbound,
		vaultSettings:       deps.VaultSettings,
		replicaSync:         newReplicaSyncService(deps.VaultRepo, deps.VaultSettings, manager),
		manager:             manager,
		supervisor:          supervisor,
		githubRepoService:   github.repos,
		githubPluginService: github.plugins,
		connRepo:            deps.ConnRepo,
		host:                host,
		assets:              compositeAssets,
		locales:             locales,
		cancel:              cancel,
	}
}

// protocolLookup returns the plugin protocol registry, or nothing.
//
// The registry of an absent manager must not be returned: a typed nil inside an interface passes a
// nil check and panics on the first call. Here the empty interface is handed back explicitly - the
// same thing NewAppAPI does.
func (r *pluginRuntime) protocolLookup() domain.ConnectionProtocolLookup {
	if r == nil || r.manager == nil {
		return nil
	}
	return r.manager.Registry()
}

// wirePeerTrust hands the trust service to both of its consumers: the plugin RPC through the
// holder, and presentation through the AppAPI. The audit sink is attached here too.
//
// All three ends are tied here and only here. One without the other would mean either a question
// nobody can close, or a button with nothing behind it, or a security decision nothing records.
func (r *pluginRuntime) wirePeerTrust(
	api *presentation.AppAPI,
	svc *usecase.PeerTrustService,
	audit usecase.PeerTrustAuditFunc,
) {
	if r == nil || api == nil || svc == nil {
		return
	}
	svc.SetAuditRecorder(audit)
	api.SetPeerTrustService(svc)
	if r.peerTrustInbound != nil {
		r.peerTrustInbound.set(svc)
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

// recordConsent stores what the user agreed to when a plugin was installed, as one grant rather
// than five separate facts (ADR-022).
func (r *pluginRuntime) recordConsent(
	ctx context.Context,
	manifest *domainplugin.Manifest,
	consent domainplugin.ConsentFlags,
) error {
	if r == nil || r.vaultSettings == nil || manifest == nil {
		return nil
	}
	return r.vaultSettings.RecordConsent(ctx, manifest.ID, domainplugin.GrantedPermissions(manifest, consent))
}

// reconcileAtUnlock brings what the vault records about each installed plugin up to date: consent
// carried forward from the boolean maps that preceded grants, and a scope folder for a plugin that
// declares one (ADR-022).
//
// It is idempotent - an existing grant and an existing scope are both left alone, so a later and
// narrower re-consent is not undone by the maps it replaced and nobody ends up with two folders.
//
// It runs on every vault unlock and is idempotent: a plugin that already has a grant is left alone,
// so a later and narrower re-consent is not undone by the maps it replaced. A failure is logged and
// nothing else stops - the vault has just opened and the user is on their way into the application;
// the next unlock tries again.
func (r *pluginRuntime) reconcileAtUnlock(ctx context.Context) {
	if r == nil || r.manager == nil || r.vaultSettings == nil {
		return
	}
	installed := r.manager.Registry().List()
	recorded, err := r.vaultSettings.EnsureConsentRecordedForAll(ctx, installed)
	if err != nil {
		log.Printf("WARNING: recording plugin consent failed after %d grants: %v", recorded, err)
	} else if recorded > 0 {
		log.Printf("recorded consent for %d plugin(s) installed before permission grants existed", recorded)
	}
	scopes, err := r.vaultSettings.EnsureScopeRootsForAll(ctx, installed)
	if err != nil {
		log.Printf("WARNING: creating plugin scope folders failed after %d: %v", scopes, err)
	} else if scopes > 0 {
		log.Printf("created %d plugin scope folder(s)", scopes)
	}
	r.syncReplicasAtUnlock(ctx, installed)
}

// syncReplicasAtUnlock pulls each replicating plugin's scope from wherever its transport reaches,
// and pushes this device's copy back (ADR-022).
//
// Unlock is the earliest it can run and the right place for it: the key that seals a replica lives
// in the vault, so before this moment there is nothing to seal with - and the plugin processes are
// already up, because they start before the vault opens.
//
// It runs off the unlock path. Every step is a plugin RPC to somebody else's server, and the user is
// on their way into the application: a laptop on a dead network would otherwise hold the window
// blank for as long as the transport takes to give up. Nothing here reports to the UI yet, so a
// failure is logged and the next unlock tries again.
func (r *pluginRuntime) syncReplicasAtUnlock(ctx context.Context, installed []domainplugin.InstalledPlugin) {
	if r.replicaSync == nil {
		return
	}
	safego.GoNamed("plugin-replica-sync", func() {
		synced, err := r.replicaSync.SyncAll(ctx, installed)
		if err != nil {
			log.Printf("WARNING: synchronising plugin scopes failed after %d: %v", synced, err)
			return
		}
		if synced > 0 {
			log.Printf("synchronised %d plugin scope(s)", synced)
		}
	})
}

// startIdleSuspender parks plugin processes that nobody is using.
//
// Extracted so newPluginRuntime stays inside its recorded budget while it gains the replication
// service. The goroutine takes the runtime's own context, so it stops when the runtime is cancelled.
func startIdleSuspender(ctx context.Context, manager *usecase.PluginManager) {
	safego.GoNamed("plugin.idleSuspender", func() {
		infrapluginlifecycle.RunIdleSuspender(ctx, manager, infrapluginlifecycle.Config{
			IdleAfter: 5 * time.Minute,
			TickEvery: time.Minute,
		})
	})
}

// newReplicaSyncService builds scope replication (ADR-022, port A).
//
// The transport is the plugin manager, because a replica travels over the plugin's own RPC, and the
// sealer is the vault's - so what the plugin carries is ciphertext it holds no key for.
func newReplicaSyncService(
	vaultRepo domain.VaultRepository,
	settings *usecase.PluginVaultSettings,
	manager *usecase.PluginManager,
) *usecase.ReplicaSyncService {
	return usecase.NewReplicaSyncService(
		vaultRepo, settings, vault.NewReplicaSealer(), usecase.NewPluginReplicaTransport(manager))
}

func (r *pluginRuntime) setSessionRecoverer(recoverer usecase.PluginSessionRecoverer) {
	if r == nil || r.supervisor == nil {
		return
	}
	r.supervisor.SetRecoverer(recoverer)
}

// wireVaultInbound gives the vault gate the two things its constructor cannot: where to write its
// audit trail, and whether the vault is open.
//
// The lock state is what makes the scope anchor safe. A scope is a position in a folder tree and
// survives a lock without effort, so an anchor that could not tell would hand a plugin access after
// the vault closed. Leaving it unwired costs a plugin its access instead of granting one it should
// not have, which is the direction a mistake here has to fall.
func wireVaultInbound(
	vaultInbound *usecase.PluginVaultInbound,
	settings *usecase.PluginVaultSettings,
	dataRoot string,
) {
	vaultInbound.SetLockState(settings)
	vaultAudit, err := auditlog.NewNDJSONVaultAuditLogger(dataRoot)
	if err != nil {
		log.Printf("WARNING: plugin vault audit logger init failed: %v", err)
		return
	}
	vaultInbound.SetAuditLogger(vaultAudit)
}
