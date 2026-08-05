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

// surfaceInboundHolder late-binds the surface service into the session RPC factory, for the same
// forced ordering discoveryInboundHolder exists for: the service needs the PluginManager in order
// to notify plugins, and that manager is built from the host this factory is part of.
//
// A miss returns ErrCapabilityDenied rather than nil — the only way to reach it is a plugin
// calling surface.* before composition finished, and a denial it can act on beats a nil
// dereference.
type surfaceInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.SurfaceInboundPort
}

func newSurfaceInboundHolder() *surfaceInboundHolder { return &surfaceInboundHolder{} }

func (h *surfaceInboundHolder) set(port domainplugin.SurfaceInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *surfaceInboundHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Handle(ctx, pluginID, method, params)
}

var _ domainplugin.SurfaceInboundPort = (*surfaceInboundHolder)(nil)

// dialogInboundHolder late-binds the dialog service, for the same forced ordering as the two
// holders above.
type dialogInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DialogInboundPort
}

func newDialogInboundHolder() *dialogInboundHolder { return &dialogInboundHolder{} }

func (h *dialogInboundHolder) set(port domainplugin.DialogInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *dialogInboundHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Handle(ctx, pluginID, method, params)
}

var _ domainplugin.DialogInboundPort = (*dialogInboundHolder)(nil)

// detailsInboundHolder late-binds the node-details service, the last of the same family.
type detailsInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DiscoveryDetailsInboundPort
}

func newDetailsInboundHolder() *detailsInboundHolder { return &detailsInboundHolder{} }

func (h *detailsInboundHolder) set(port domainplugin.DiscoveryDetailsInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *detailsInboundHolder) PublishDetails(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.PublishDetails(ctx, pluginID, params)
}

var _ domainplugin.DiscoveryDetailsInboundPort = (*detailsInboundHolder)(nil)

// discoveryEmitHolder late-binds the frontend emit callback the same way discoveryInboundHolder
// late-binds the inbound port, and for the same forced ordering: the emit coalescer is built inside
// the plugin runtime, and the AppAPI that owns the Wails context is built afterwards from it.
//
// A miss is a no-op, not an error. Between composition and wireEmbed there is no window in which a
// tree can change — no plugin has been told to observe anything yet — and a redraw nobody can
// receive is nothing to report.
type discoveryEmitHolder struct {
	mu   sync.Mutex
	emit func(connectionID, nodeID string)
}

func newDiscoveryEmitHolder() *discoveryEmitHolder { return &discoveryEmitHolder{} }

func (h *discoveryEmitHolder) set(emit func(connectionID, nodeID string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emit = emit
}

// notify runs the callback outside the lock: it reaches into the Wails runtime, and holding a mutex
// across that would serialize every tree update behind an unknown amount of presentation work.
func (h *discoveryEmitHolder) notify(connectionID, nodeID string) {
	h.mu.Lock()
	emit := h.emit
	h.mu.Unlock()
	if emit != nil {
		emit(connectionID, nodeID)
	}
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
	sessionRegistry := newSessionRegistryHolder()
	// The discovery service cannot exist yet: it needs the PluginManager, which needs the host,
	// which needs this session RPC factory. The holder closes that cycle the same way
	// sessionRegistryHolder does — the port is resolved at CALL time, and a plugin process cannot
	// call discovery.publish before it has been started by the very manager that fills it in.
	discoveryInbound := newDiscoveryInboundHolder()
	surfaceInbound := newSurfaceInboundHolder()
	dialogInbound := newDialogInboundHolder()
	detailsInbound := newDetailsInboundHolder()

	hostCfg := infraplugin.HostConfig{
		DataRoot:          dataRoot,
		Portable:          portableRuntime,
		Vault:             vaultInbound,
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, embedInbound, discoveryInbound, surfaceInbound, dialogInbound, detailsInbound, sessionAuthorizer),
		Events:            eventBus,
		Views:             viewInbound,
		Tunnel:            dynamicForward,
		SessionAuthorizer: sessionAuthorizer,
		Audit:             pluginAudit.RPCRecorder(),
		// The channel purpose backends and everything they need are assembled in
		// main_channels.go: constructing them is its own reason to change, separate from wiring
		// the plugin runtime.
		ChannelResolverFor: newChannelResolverFor(pluginAudit.ChannelFunc(), embedTunnels, sessionRegistry),
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
	// shape the channel bus uses. Tree changes reach the frontend through discoveryEmit, filled in
	// by wireEmbed once the AppAPI exists.
	discoveryStore := usecase.NewDiscoveryStore()
	discoveryObserver := usecase.NewDiscoveryObserver(registry, manager)
	discoveryEmit := newDiscoveryEmitHolder()
	// Pace is built before the leader because both the publish path and connection teardown need
	// it, and teardown lives on the leader.
	discoveryPace := usecase.NewDiscoveryPace(
		usecase.NewDiscoveryPublishLimiter(nil),
		usecase.NewDiscoveryEmitCoalescer(discoveryEmit.notify, nil, nil),
	)
	// The leader owns the plugin lifecycle for discovery, and it is the only thing that can: a
	// discovery plugin draws under a core SSH connection it does not own, so the session bridge —
	// which binds a plugin to the session it provides — never binds it, and every publish would be
	// refused by the IDOR check. registry and manager are constructor arguments so this cannot be
	// left unwired without failing to compile.
	// The last argument is discoveryEmit.notify, not nil. A handover marks every branch stale and
	// the backend starts refusing actions inside them from that moment; with no callback the frontend
	// was never told, so the rows still looked live and a click came back with "branch is stale or
	// failed" and nothing on screen explaining why. It bypasses the coalescer deliberately — see
	// NewDiscoveryLeader.
	discoveryLeader := usecase.NewDiscoveryLeader(sessionRegistry, registry, manager, discoveryStore, discoveryObserver, discoveryPace, discoveryEmit.notify)
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

	// Surfaces (ADR-015). The presenter is left nil here and pushed in by wireEmbed once the AppAPI
	// exists, exactly like discoveryEmit: until then a surface's output goes nowhere, which is the
	// correct behaviour for a UI that is not up yet.
	surfaceRegistry := usecase.NewSurfaceRegistry()
	surfaceService := usecase.NewSurfaceService(
		surfaceRegistry,
		nil,
		usecase.NewSurfaceNotifier(manager),
		sessionRegistry,
		registry.UICapabilities,
		pluginAudit.SurfaceFunc(),
	)
	surfaceInbound.set(surfaceService)

	// Dialogs share the surface capability lookup: both ask the same manifest the same question.
	dialogService := usecase.NewDialogService(
		usecase.NewDialogRegistry(),
		nil,
		usecase.NewDialogNotifier(manager),
		registry.UICapabilities,
	)
	dialogInbound.set(dialogService)

	// Node details (ADR-015 §3): a discovery verb addressing a node and a ui verb drawing a panel,
	// which is why it needs both grants and reuses both services' dependencies.
	detailsService := usecase.NewDiscoveryDetailsService(
		discoveryStore,
		discoveryLeader,
		manager,
		pluginAudit.DiscoveryFunc(),
		registry.UICapabilities,
		discoveryPace,
	)
	detailsInbound.set(detailsService)
	// A restarted plugin is told the whole observed set again; without this the level-triggered
	// contract silently degrades into an edge-triggered one (ADR-014 §data flow).
	manager.SetProcessStartedHandler(discoveryObserver.PluginStarted)
	// The other end of the same lifecycle: a plugin the user disabled or uninstalled loses its
	// subtree at once, under every connection, without touching its neighbours' (ADR-014).
	manager.SetProcessStoppedHandler(func(pluginID string) {
		discoveryService.ClearPlugin(pluginID)
		// A surface is a live stream, not a snapshot: a stopped plugin has no way to resume
		// writing into a tab it no longer remembers, so the tabs go with it rather than being
		// left showing a stream nobody is producing.
		surfaceService.CloseSurfacesForPlugin(pluginID)
	})
	// A crash and an idle suspension get the opposite treatment on purpose: the process comes back
	// — restarted by the supervisor, or on the next activation — and the replayed observed set
	// refills the tree, so the branches are marked stale rather than deleted. Marking also refuses
	// actions inside them, which beats dispatching into a dead process and waiting out the ack.
	// Surfaces take the opposite treatment from discovery branches on both transitions. A branch is
	// a snapshot that a restarted plugin refills from the replayed observed set; a surface is a
	// stream with no such replay, so a process that is gone takes its tabs with it either way.
	manager.SetProcessCrashedHandler(func(pluginID string) {
		discoveryService.MarkPluginStale(pluginID)
		surfaceService.CloseSurfacesForPlugin(pluginID)
		dialogService.CancelForPlugin(pluginID)
	})
	manager.SetProcessSuspendedHandler(func(pluginID string) {
		discoveryService.MarkPluginStale(pluginID)
		surfaceService.CloseSurfacesForPlugin(pluginID)
		dialogService.CancelForPlugin(pluginID)
	})
	// And the end of that story: once the supervisor stops trying, stale stops being true. The
	// branches become error with a reason, so "restarting" and "given up" are not the same grey
	// subtree (ADR-014 §Leading session / plan п.13).
	supervisor.SetGaveUpHandler(discoveryService.MarkPluginUnrecoverable)
	// And what makes that story reachable at all: a plugin drawing a subtree is in use, even though
	// it holds no session and owns no view panel. Without this the idle sweeper reclaimed it after
	// five quiet minutes and the supervisor declined to restart it, so its branches went stale and
	// stayed there — the promise in ADR-014 §Leading session with nothing behind it.
	// An open surface counts as "in use" for the same reason a discovery binding does, and the trap
	// is the same one: a plugin streaming a log into a tab the user is watching is silent on the
	// RPC channel, which is exactly what idle looks like from here.
	manager.SetPluginRetentionChecker(func(pluginID string) bool {
		return discoveryLeader.HoldsBindings(pluginID) || surfaceService.HoldsSurfaces(pluginID)
	})

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
		surfaces:            surfaceService,
		dialogs:             dialogService,
		nodeDetails:         detailsService,
		discoveryEmit:       discoveryEmit,
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
	if r.discoveryService != nil {
		api.SetDiscoveryService(r.discoveryService)
	}
	if r.discoveryEmit != nil {
		r.discoveryEmit.set(api.EmitDiscoveryTreeChanged)
	}
	// Surfaces are wired in both directions here (ADR-015): the AppAPI is the presenter the use
	// case pushes tabs and bytes to, and the use case is what the AppAPI's own handlers call when
	// the user types into one or closes it. The close cascade goes through the session manager,
	// beside the channel bus it runs next to.
	if r.surfaces != nil {
		r.surfaces.SetPresenter(presentation.NewSurfacePresenter(api))
		api.SetSurfaceService(r.surfaces)
		api.Sessions().SetSurfaces(r.surfaces)
	}
	if r.dialogs != nil {
		r.dialogs.SetPresenter(presentation.NewDialogPresenter(api))
		api.SetDialogService(r.dialogs)
	}
	if r.nodeDetails != nil {
		r.nodeDetails.SetEmitter(api.EmitNodeDetailsChanged)
		api.SetNodeDetailsService(r.nodeDetails)
	}
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
