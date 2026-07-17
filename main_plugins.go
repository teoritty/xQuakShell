package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/auditlog"
	infracache "ssh-client/internal/infra/cache"
	"ssh-client/internal/infra/plugin/capability"
	infragithub "ssh-client/internal/infra/github"
	infrapluginembed "ssh-client/internal/infra/embed"
	infraplugin "ssh-client/internal/infra/plugin"
	infrapluginassets "ssh-client/internal/infra/plugin/assets"
	infrapluginbundle "ssh-client/internal/infra/plugin/bundle"
	infrapluginlifecycle "ssh-client/internal/infra/plugin/lifecycle"
	infrapersistence "ssh-client/internal/infra/persistence"
	infraportable "ssh-client/internal/infra/portable"
	"ssh-client/internal/pkg/ratelimit"
	"ssh-client/internal/pkg/safego"
	presentation "ssh-client/internal/presentation/wails"
	"ssh-client/internal/usecase"
)

type pluginRuntime struct {
	inbound             *usecase.PluginSessionInbound
	embedInbound        *usecase.PluginEmbedInbound
	embedTunnels        *usecase.EmbedTunnelService
	dynamicForward      *usecase.DynamicForwardCoordinator
	embedBridge         *usecase.PluginEmbedBridge
	channelBus          *capability.ChannelBus
	viewInbound         *usecase.PluginViewInbound
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

	hostCfg := infraplugin.HostConfig{
		DataRoot:          dataRoot,
		Portable:          portableRuntime,
		Vault:             vaultInbound,
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, embedInbound, sessionAuthorizer),
		Events:            eventBus,
		Views:             viewInbound,
		Tunnel:            dynamicForward,
		SessionAuthorizer: sessionAuthorizer,
		Audit:             pluginAudit.RPCRecorder(),
		// The channel purpose backends and everything they need are assembled in
		// main_channels.go: constructing them is its own reason to change, separate from wiring
		// the plugin runtime.
		ChannelResolverFor: newChannelResolverFor(pluginAudit.ChannelFunc(), embedTunnels),
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

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(deps.ExeDir, dataRoot))
	if err := manager.DiscoverPlugins(discovery.Discover); err != nil {
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
		dynamicForward:      dynamicForward,
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
	api.Sessions().SetChannelBus(r.channelBus)
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
