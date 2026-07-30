package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/auditlog"
	infracache "xquakshell/internal/infra/cache"
	"xquakshell/internal/infra/plugin/capability"
	infragithub "xquakshell/internal/infra/github"
	infrapluginembed "xquakshell/internal/infra/embed"
	infraplugin "xquakshell/internal/infra/plugin"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	infrapluginbundle "xquakshell/internal/infra/plugin/bundle"
	infrapluginlifecycle "xquakshell/internal/infra/plugin/lifecycle"
	infrapersistence "xquakshell/internal/infra/persistence"
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

// discoveryInboundHolder makes the discovery service reachable from the session RPC factory despite
// being constructed after it, the same push-me-later shape sessionRegistryHolder uses.
//
// The ordering is forced: SessionRPC is part of HostConfig, HostConfig builds the ProcessHost, the
// ProcessHost is what PluginManager drives, and the discovery service needs that manager to notify
// and call plugins. Something has to be late-bound, and this is the smallest something.
//
// A miss returns ErrCapabilityDenied rather than nil: the only way to reach it is a plugin process
// publishing before composition finished, and a denial the plugin can act on beats a nil
// dereference. It is exactly how PluginSessionRPCHandler treats every other unwired inbound port.
type discoveryInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DiscoveryInboundPort
}

func newDiscoveryInboundHolder() *discoveryInboundHolder { return &discoveryInboundHolder{} }

func (h *discoveryInboundHolder) set(port domainplugin.DiscoveryInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *discoveryInboundHolder) Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Publish(ctx, pluginID, params)
}

var _ domainplugin.DiscoveryInboundPort = (*discoveryInboundHolder)(nil)

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

	var manager *usecase.PluginManager
	var supervisor *usecase.PluginSupervisor
	dynamicForward := usecase.NewDynamicForwardCoordinator(nil, nil)
	eventBus := usecase.NewPluginEventBus(registry, func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
		if manager == nil {
			return nil
		}
		return manager.NotifyProcess(ctx, pluginID, sessionID, method, params)
	})

	channelBus := capability.NewChannelBus()
	channelCloseNotify := newChannelCloseNotifiers()
	sessionRegistry := newSessionRegistryHolder()
	// The discovery service cannot exist yet: it needs the PluginManager, which needs the host,
	// which needs this session RPC factory. The holder closes that cycle the same way
	// sessionRegistryHolder does — the port is resolved at CALL time, and a plugin process cannot
	// call discovery.publish before it has been started by the very manager that fills it in.
	discoveryInbound := newDiscoveryInboundHolder()

	hostCfg := infraplugin.HostConfig{
		DataRoot:          dataRoot,
		Portable:          portableRuntime,
		Vault:             vaultInbound,
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, embedInbound, discoveryInbound, sessionAuthorizer),
		Events:            eventBus,
		Views:             viewInbound,
		Tunnel:            dynamicForward,
		SessionAuthorizer: sessionAuthorizer,
		Audit:             pluginAudit.RPCRecorder(),
		// The channel purpose backends and everything they need are assembled in
		// main_channels.go: constructing them is its own reason to change, separate from wiring
		// the plugin runtime.
		ChannelResolverFor:         newChannelResolverFor(pluginAudit.ChannelFunc(), embedTunnels, channelCloseNotify, sessionRegistry),
		AttachChannelCloseNotifier: channelCloseNotify.attach,
		ChannelAudit:       pluginAudit.ChannelFunc(),
		ChannelBus:         channelBus,
		OnCrash: func(pluginID, sessionID string) {
			if manager != nil {
				manager.OnProcessCrashed(pluginID, sessionID)
				if supervisor != nil {
					supervisor.HandleCrash(pluginID, sessionID)
				}
			}
		},
		OnPluginActivity: func(pluginID string) {
			if manager != nil {
				manager.TouchActivity(pluginID)
			}
		},
	}
	host := infraplugin.NewProcessHost(hostCfg)
	manager = usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:       registry,
		Host:           host,
		LoadBundle:     infraplugin.LoadPluginSource,
		InstallBundle:  infraplugin.InstallFromSource,
		InstallRoot:    dataRoot,
		PortableData:   portableData,
		Bundle:         infrapluginbundle.BundleAdapter{},
		Portable:       portableRuntime,
		PluginSettings: deps.VaultSettings,
		StartAudit:     pluginAudit.StartFunc(),
	})
	manager.SetOutboundAuthAudit(pluginAudit.OutboundAuthFunc())
	manager.SetEventBus(eventBus)
	manager.SetPluginSettings(deps.VaultSettings)
	manager.SetConnectionChecker(deps.ConnRepo)
	supervisor = usecase.NewPluginSupervisor(manager)
	viewRelay := usecase.NewPluginViewRelay(manager, registry)
	eventBus.SetSessionActiveChecker(func(pluginID string) bool {
		return manager.ActiveSessionCount(pluginID) > 0
	})

	// Discovery subtrees (ADR-014). The observer and the leader need each other — the observer must
	// resolve a connection to its leading session, the leader must resend the observed set on
	// handover — so the observer is built first and the leader pushed into it, the same late-binding
	// shape the channel bus uses. onChange is nil until the presentation layer that emits tree
	// updates to the frontend is wired.
	discoveryStore := usecase.NewDiscoveryStore()
	discoveryObserver := usecase.NewDiscoveryObserver(registry, manager)
	// Pace is built before the leader because both the publish path and connection teardown need
	// it, and teardown lives on the leader.
	discoveryPace := usecase.NewDiscoveryPace(
		usecase.NewDiscoveryPublishLimiter(nil),
		usecase.NewDiscoveryEmitCoalescer(nil, nil, nil),
	)
	discoveryLeader := usecase.NewDiscoveryLeader(sessionRegistry, discoveryStore, discoveryObserver, discoveryPace, nil)
	discoveryObserver.SetLeader(discoveryLeader)
	discoveryService := usecase.NewDiscoveryService(
		discoveryStore,
		discoveryObserver,
		// The registry supplies the declared icon IDs: an iconId no manifest registered is dropped
		// from the node rather than failing the publish (ADR-014 §Security model).
		usecase.NewDiscoveryPublishRouter(discoveryStore, discoveryObserver, discoveryLeader, discoveryPace, registry),
		usecase.NewDiscoveryInvoker(discoveryStore, discoveryLeader, manager, pluginAudit.DiscoveryFunc()),
	)
	// Now the plugin->host half can be reached: discovery.publish arrives through the session RPC
	// handler, which authorizes the sessionId before anything here sees the snapshot.
	discoveryInbound.set(discoveryService)
	// A restarted plugin is told the whole observed set again; without this the level-triggered
	// contract silently degrades into an edge-triggered one (ADR-014 §data flow).
	manager.SetProcessStartedHandler(discoveryObserver.PluginStarted)
	// The other end of the same lifecycle: a plugin the user disabled or uninstalled loses its
	// subtree at once, under every connection, without touching its neighbours' (ADR-014).
	manager.SetProcessStoppedHandler(discoveryService.ClearPlugin)

	pluginDiscovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(deps.ExeDir, dataRoot))
	if err := manager.DiscoverPlugins(pluginDiscovery.Discover); err != nil {
		log.Printf("WARNING: plugin discovery failed: %v", err)
	}

	if err := infrapersistence.EnsureGitHubReposFile(dataRoot); err != nil {
		log.Printf("WARNING: github repos file init failed: %v", err)
	}

	githubCache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	githubRepoStorage, err := infrapersistence.NewFileGitHubRepositoryStorage(dataRoot)
	if err != nil {
		log.Printf("WARNING: github repo storage init failed: %v", err)
	}
	githubClient := infragithub.NewUseCaseClient(infragithub.NewClient())
	tempDir := ""
	if portableData != nil {
		if dir, err := portableData.EnsureTempDir(); err == nil {
			tempDir = dir
		} else {
			log.Printf("WARNING: portable temp dir unavailable for GitHub downloads: %v", err)
		}
	}
	githubDownloader := infraplugin.NewBinaryDownloader(infragithub.NewClient(), tempDir)
	githubStager := infraplugin.NewGitHubPluginStager(tempDir)

	var githubRepoService *usecase.GitHubRepositoryService
	var githubPluginService *usecase.GitHubPluginService
	if githubRepoStorage != nil {
		githubRepoService = usecase.NewGitHubRepositoryService(githubRepoStorage, githubCache)
		githubPluginService = usecase.NewGitHubPluginService(
			githubClient,
			githubDownloader,
			githubStager,
			infraplugin.InstallMetaWriter{},
			githubCache,
			manager,
			githubRepoStorage,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())
	safego.GoNamed("plugin.idleSuspender", func() {
		infrapluginlifecycle.RunIdleSuspender(ctx, manager, infrapluginlifecycle.Config{
			IdleAfter: 5 * time.Minute,
			TickEvery: time.Minute,
		})
	})

	pluginAssets := infrapluginassets.NewHandler(infrapluginassets.PluginRegistryUIRootResolver(func(id string) (domainplugin.InstalledPlugin, error) {
		return registry.Get(id)
	}))
	embedBroker := infrapluginembed.NewBrokerHandler(embedTunnels, func(pluginID string) (string, error) {
		p, err := registry.Get(pluginID)
		if err != nil {
			return "", err
		}
		return filepath.Join(p.RootDir, "ui"), nil
	})
	compositeAssets := infrapluginembed.NewCompositeHandler(pluginAssets, embedBroker)

	// The composite handler is also served through the Wails asset server (wails.localhost), but
	// on Windows/WebView2 that host serves HTTP only — it does NOT proxy ws:// upgrades, so the
	// embed tunnel WebSocket can never reach the broker there. Serve the same handler from a real
	// loopback listener and point the embed UI/tunnel URLs at it (SetBaseURL), so the iframe loads
	// its assets AND opens its ws:// tunnel against one same-origin host that actually accepts the
	// upgrade. Best-effort: if the listener cannot bind, embed sessions degrade rather than crash.
	if ln, lnErr := net.Listen("tcp", "127.0.0.1:0"); lnErr != nil {
		slog.Warn("embed: loopback broker listener unavailable; embed tunnels disabled", "err", lnErr)
	} else {
		// No WriteTimeout/ReadTimeout: this server also serves WebSocket upgrades for embed
		// tunnels (broker_handler.go), and either deadline would sever those long-lived
		// connections. ReadHeaderTimeout and IdleTimeout are safe for upgraded connections and
		// still bound unauthenticated connections that never send a request.
		srv := &http.Server{
			Handler:           compositeAssets,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		safego.GoNamed("embed.loopbackBroker", func() { _ = srv.Serve(ln) })
		embedTunnels.SetBaseURL("http://" + ln.Addr().String())
		slog.Info("embed: loopback broker listening", "addr", ln.Addr().String())
	}

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
		discoveryService:    discoveryService,
		discoveryLeader:     discoveryLeader,
		viewInbound:         viewInbound,
		viewRelay:           viewRelay,
		vaultInbound:        vaultInbound,
		vaultSettings:       deps.VaultSettings,
		manager:             manager,
		supervisor:          supervisor,
		githubRepoService:   githubRepoService,
		githubPluginService: githubPluginService,
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
