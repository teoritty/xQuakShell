package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/auditlog"
	infracache "ssh-client/internal/infra/cache"
	infragithub "ssh-client/internal/infra/github"
	infraplugin "ssh-client/internal/infra/plugin"
	infrapluginassets "ssh-client/internal/infra/plugin/assets"
	infrapluginbundle "ssh-client/internal/infra/plugin/bundle"
	infrapluginlifecycle "ssh-client/internal/infra/plugin/lifecycle"
	infrapersistence "ssh-client/internal/infra/persistence"
	infraportable "ssh-client/internal/infra/portable"
	"ssh-client/internal/usecase"
)

type pluginRuntime struct {
	inbound             *usecase.PluginSessionInbound
	viewInbound         *usecase.PluginViewInbound
	viewRelay           *usecase.PluginViewRelay
	vaultInbound        *usecase.PluginVaultInbound
	vaultSettings       *usecase.PluginVaultSettings
	manager             *usecase.PluginManager
	supervisor          *usecase.PluginSupervisor
	githubRepoService   *usecase.GitHubRepositoryService
	githubPluginService *usecase.GitHubPluginService
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

func newPluginRuntime(dataRoot string, deps pluginRuntimeDeps) *pluginRuntime {
	inbound := usecase.NewPluginSessionInbound()
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
	eventBus := usecase.NewPluginEventBus(registry, func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
		if manager == nil {
			return nil
		}
		return manager.NotifyProcess(ctx, pluginID, sessionID, method, params)
	})

	hostCfg := infraplugin.HostConfig{
		DataRoot:          dataRoot,
		Portable:          portableRuntime,
		Vault:             vaultInbound,
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, sessionAuthorizer),
		Events:            eventBus,
		Views:             viewInbound,
		SessionAuthorizer: sessionAuthorizer,
		Audit:             pluginAudit.RPCRecorder(),
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
		Bundle:         infrapluginbundle.BundleAdapter{},
		Portable:       portableRuntime,
		PluginSettings: deps.VaultSettings,
		StartAudit:     pluginAudit.StartFunc(),
	})
	manager.SetEventBus(eventBus)
	manager.SetPluginSettings(deps.VaultSettings)
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
	githubClient := infragithub.NewClient()
	githubDownloader := infraplugin.NewBinaryDownloader(githubClient)

	var githubRepoService *usecase.GitHubRepositoryService
	var githubPluginService *usecase.GitHubPluginService
	if githubRepoStorage != nil {
		githubRepoService = usecase.NewGitHubRepositoryService(githubRepoStorage, githubCache)
		githubPluginService = usecase.NewGitHubPluginService(
			githubClient,
			githubDownloader,
			nil,
			githubCache,
			manager,
			githubRepoStorage,
			dataRoot,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go infrapluginlifecycle.RunIdleSuspender(ctx, manager, infrapluginlifecycle.Config{
		IdleAfter: 5 * time.Minute,
		TickEvery: time.Minute,
	})

	return &pluginRuntime{
		inbound:             inbound,
		viewInbound:         viewInbound,
		viewRelay:           viewRelay,
		vaultInbound:        vaultInbound,
		vaultSettings:       deps.VaultSettings,
		manager:             manager,
		supervisor:          supervisor,
		githubRepoService:   githubRepoService,
		githubPluginService: githubPluginService,
		assets: infrapluginassets.NewHandler(infrapluginassets.PluginRegistryUIRootResolver(func(id string) (domainplugin.InstalledPlugin, error) {
			return registry.Get(id)
		})),
		cancel: cancel,
	}
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

func (r *pluginRuntime) setSessionRecoverer(recoverer usecase.PluginSessionRecoverer) {
	if r == nil || r.supervisor == nil {
		return
	}
	r.supervisor.SetRecoverer(recoverer)
}
