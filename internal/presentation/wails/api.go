package wails

import (
	"context"
	"log/slog"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/presentation/logwindow"
	"xquakshell/internal/usecase"
)

// AppAPI is the Wails-bound struct that exposes all backend methods to the frontend.
type AppAPI struct {
	ctx                         context.Context
	vaultRepo                   domain.VaultRepository
	vaultSvc                    *usecase.VaultService
	keys                        *usecase.KeyManagerService
	keyDeploy                   *usecase.KeyDeployService
	migrator                    domain.VaultMigrator
	migrationDeps               domain.MigrationDeps
	sessions                    *usecase.SessionManager
	settingsSvc                 *usecase.SettingsService
	auditSvc                    *usecase.AuditService
	transferSvc                 *usecase.TransferService
	transferPlanner             *usecase.TransferPlanner
	remoteOpSvc                 *usecase.RemoteOpService
	cancels                     *usecase.CancelRegistry // shared by planner, executor and remote ops: one id space, one registry
	hostKeys                    *usecase.HostKeyService
	peerTrust                   *usecase.PeerTrustService
	remoteFS                    *usecase.RemoteFSService
	localFS                     *usecase.LocalFSService
	portableData                domain.PortableDataStore
	puttyImport                 *usecase.PuTTYImportService
	sshConfigImport             *usecase.SSHConfigImportService
	lockout                     domain.LockoutManager
	pingMgr                     *usecase.PingManager
	plugins                     *usecase.PluginManager
	viewRelay                   *usecase.PluginViewRelay
	githubRepoService           *usecase.GitHubRepositoryService
	githubPluginService         *usecase.GitHubPluginService
	pluginCatalog               *usecase.PluginCatalogService
	pluginVaultGrant            func(pluginID string) error
	pluginAuthGrant             func(pluginID string) error
	pluginTunnelGrant           func(pluginID string) error
	pluginMultiSessionGrant     func(pluginID string) error
	pluginArbitraryNetworkGrant func(pluginID string) error
	discovery                   DiscoveryTreeService
	surfaces                    SurfaceCommands
	localTerminals              LocalTerminalCommands
	localShells                 domain.ShellCatalog
	dialogs                     DialogCommands
	nodeDetails                 NodeDetailsService
	embedBridge                 *usecase.PluginEmbedBridge
	forwardRules                *usecase.ForwardRuleValidator
	logWindow                   *logwindow.Manager
	logLevel                    domain.LogLevelController
	unlockThrottle              domain.UnlockThrottle
	recovery                    domain.VaultRecovery
	recoveryThrottle            *domain.UnlockThrottle
	recoveryAudit               *usecase.RecoveryAuditRecorder
	pendingRecovery             pendingRecoveryKey
	updateSvc                   *usecase.UpdateService
	locales                     domain.LocaleCatalog
	localeBroadcast             LocaleBroadcaster
}

// NewAppAPI creates a new AppAPI with the given dependencies.
func NewAppAPI(
	vaultRepo domain.VaultRepository,
	connRepo domain.ConnectionRepository,
	identRepo domain.IdentityRepository,
	passwordRepo domain.PasswordRepository,
	knownHosts domain.KnownHostsRepository,
	sshFactory domain.SSHClientFactory,
	sshSession usecase.SSHSessionDeps,
	sessionConnectors []domain.SessionConnector,
	auditLogRepo domain.AuditLogRepository,
	lockoutMgr domain.LockoutManager,
	hostFS domain.HostFileSystem,
	hostLauncher domain.HostAppLauncher,
	isHiddenLocal func(fullPath, name string) bool,
	portableData domain.PortableDataStore,
	trackerFactory domain.CommandLineTrackerFactory,
	sanitizerFactory domain.AuditInputSanitizerFactory,
	puttyImporter domain.PuTTYImporter,
	sshConfigImporter domain.SSHConfigImporter,
	pluginMgr *usecase.PluginManager,
	pluginInbound *usecase.PluginSessionInbound,
	pluginViewInbound *usecase.PluginViewInbound,
	pluginVaultInbound *usecase.PluginVaultInbound,
	logStream domain.LogStream,
	pluginSessionAudit domainplugin.SessionAuditor,
	pingLimiter domain.ConcurrencyLimiter,
	transferLimiter domain.ConcurrencyLimiter,
	forwardConnLimiterFactory func() domain.ConcurrencyLimiter,
	pinger domain.Pinger,
	sshAuth *usecase.SSHAuthWiring,
	logLevel domain.LogLevelController,
) *AppAPI {
	pingMgr := usecase.NewPingManager(connRepo, domain.DefaultPingSettings(), pingLimiter, pinger)
	var pluginFieldsSvc *usecase.PluginFieldsService
	var protocolLookup domain.ConnectionProtocolLookup
	if pluginMgr != nil {
		pluginFieldsSvc = usecase.NewPluginFieldsService(vaultRepo, pluginMgr.Registry())
		protocolLookup = pluginMgr.Registry()
		pingMgr.SetProtocolLookup(protocolLookup)
	}
	vaultSvc := usecase.NewVaultService(usecase.VaultServiceConfig{
		ConnRepo:       connRepo,
		PasswordRepo:   passwordRepo,
		IdentRepo:      identRepo,
		PluginFields:   pluginFieldsSvc,
		PingMgr:        pingMgr,
		ProtocolLookup: protocolLookup,
	})
	api := &AppAPI{
		vaultRepo:       vaultRepo,
		vaultSvc:        vaultSvc,
		portableData:    portableData,
		puttyImport:     usecase.NewPuTTYImportService(connRepo, identRepo, puttyImporter),
		sshConfigImport: usecase.NewSSHConfigImportService(connRepo, identRepo, sshConfigImporter),
		lockout:         lockoutMgr,
		pingMgr:         pingMgr,
		plugins:         pluginMgr,
		settingsSvc:     usecase.NewSettingsService(vaultRepo, lockoutMgr, pingMgr),
		logLevel:        logLevel,
	}

	smCfg := usecase.SessionManagerConfig{
		ConnRepo:               connRepo,
		VaultRepo:              vaultRepo,
		IdentRepo:              identRepo,
		PasswordRepo:           passwordRepo,
		KnownHosts:             knownHosts,
		SSHFactory:             sshFactory,
		PassphraseCache:        sshSession.PassphraseCache,
		HostKeyCallbackBuilder: sshSession.HostKeyCallbackBuilder,
		JumpTransportBuilder:   sshSession.JumpTransportBuilder,
		Keys:                   sshSession.Keys,
		PTYBridgeFactory:       sshSession.PTYBridgeFactory,
		SFTPClientFactory:      sshSession.SFTPClientFactory,
		Connectors:             sessionConnectors,
		PluginBridge: usecase.NewPluginSessionBridge(usecase.PluginSessionBridgeConfig{
			Plugins: pluginMgr,
			Fields:  pluginFieldsSvc,
			Audit:   pluginSessionAudit,
		}),
		OnStateChange:             api.onSessionStateChange,
		OnStreamReady:             api.onStreamReady,
		PassphraseReq:             api.onPassphraseRequest,
		HostKeyRequest:            api.onHostKeyRequest,
		ForwardConnLimiterFactory: forwardConnLimiterFactory,
	}
	if sshAuth != nil && sshAuth.Enabled() {
		smCfg.AuthProvider = sshAuth.Provider
		smCfg.AuthMethodBuilder = sshAuth.Builder
		smCfg.AuthAttempts = sshAuth.Attempts
		smCfg.AuthLookup = sshAuth.Lookup
		smCfg.AuthStarter = sshAuth.Starter
		smCfg.AuthGrantReader = sshAuth.GrantReader
	}
	api.wireSessionsAndKeys(usecase.NewSessionManager(smCfg), vaultRepo, sshSession, auditLogRepo)
	if pluginInbound != nil && api.sessions.PluginBridge() != nil {
		pluginInbound.SetHandler(api.sessions.PluginBridge())
	}
	if pluginVaultInbound != nil && api.sessions.PluginBridge() != nil {
		pluginVaultInbound.SetAuthorizer(api.sessions.PluginBridge())
	}
	if pluginViewInbound != nil {
		pluginViewInbound.SetHandler(api)
	}

	api.auditSvc = usecase.NewAuditService(auditLogRepo, api.settingsSvc, api.sessions, connRepo, trackerFactory, sanitizerFactory)
	api.cancels = usecase.NewCancelRegistry()
	api.transferSvc = usecase.NewTransferService(api.sessions, api.settingsSvc, hostFS, transferLimiter, api.cancels)
	api.transferPlanner = usecase.NewTransferPlanner(api.sessions, hostFS, api.cancels)
	api.remoteOpSvc = usecase.NewRemoteOpService(api.sessions, api.cancels)
	api.hostKeys = usecase.NewHostKeyService(knownHosts, api.sessions)
	api.remoteFS = usecase.NewRemoteFSService(api.sessions)
	api.localFS = usecase.NewLocalFSService(usecase.LocalFSServiceConfig{
		HostFS:   hostFS,
		Launcher: hostLauncher,
		IsHidden: isHiddenLocal,
	})
	api.logWindow = logwindow.NewManager(logStream, api.settingsSvc, api.emitDebugLogWindowChanged)

	return api
}

func (a *AppAPI) emitDebugLogWindowChanged(enabled bool) {
	if a == nil || a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventDebugLogWindowChanged, map[string]bool{"enabled": enabled})
}

// SyncDebugLogWindow starts or stops the debug log viewer subprocess.
func (a *AppAPI) SyncDebugLogWindow(enabled bool) {
	if a == nil || a.logWindow == nil {
		return
	}
	a.logWindow.SyncEnabled(a.reqCtx(), enabled)
}

// StopDebugLogWindow closes the debug log viewer subprocess.
func (a *AppAPI) StopDebugLogWindow() {
	if a == nil || a.logWindow == nil {
		return
	}
	a.logWindow.Stop()
}

// Sessions exposes the session manager for composition-root wiring.
func (a *AppAPI) Sessions() *usecase.SessionManager {
	return a.sessions
}

// SettingsService exposes the settings service for composition-root wiring, mirroring Sessions.
func (a *AppAPI) SettingsService() *usecase.SettingsService {
	return a.settingsSvc
}

// SetForwardRuleValidator wires forward rule validation for save and connect paths.
func (a *AppAPI) SetForwardRuleValidator(v *usecase.ForwardRuleValidator) {
	a.forwardRules = v
	if a.vaultSvc != nil {
		a.vaultSvc.SetForwardRuleValidator(v)
	}
}

// SetEmbedBridge wires embed viewport/activity forwarding.
func (a *AppAPI) SetEmbedBridge(bridge *usecase.PluginEmbedBridge) {
	a.embedBridge = bridge
}

// OnEmbedReady emits SessionEmbedReady when a plugin registers an embed surface.
func (a *AppAPI) OnEmbedReady(desc domain.SessionEmbedDescriptor) {
	if a == nil || a.ctx == nil {
		slog.Debug("embed: OnEmbedReady skipped, nil app ctx", "pluginId", desc.PluginID, "sessionId", desc.SessionID)
		return
	}
	slog.Debug("embed: emitting SessionEmbedReady to frontend", "pluginId", desc.PluginID, "sessionId", desc.SessionID, "uiUrl", desc.UIUrl, "tunnelUrl", desc.TunnelUrl)
	wailsrt.EventsEmit(a.ctx, EventSessionEmbedReady, SessionEmbedToDTO(desc))
}

// SetGitHubServices wires GitHub repository and plugin services.
func (a *AppAPI) SetGitHubServices(repoSvc *usecase.GitHubRepositoryService, pluginSvc *usecase.GitHubPluginService) {
	a.githubRepoService = repoSvc
	a.githubPluginService = pluginSvc
}

// SetPluginCatalog wires the source-aware catalog router.
//
// It is set separately from SetGitHubServices rather than added to it: the catalog outlives the
// forge services conceptually - it is what a second, non-forge source is reached through - and
// bundling the two would make the marketplace's availability depend on GitHub storage having
// opened successfully.
func (a *AppAPI) SetPluginCatalog(catalog *usecase.PluginCatalogService) {
	a.pluginCatalog = catalog
}

// SetPluginManager wires the plugin manager for handler delegation.
func (a *AppAPI) SetPluginManager(mgr *usecase.PluginManager) {
	a.plugins = mgr
}

// Lifecycle: call once on app startup. Starts the idle lockout monitor when a lockout manager is configured.
// Ping monitoring is started from UnlockVault when settings are applied, not here.
func (a *AppAPI) SetContext(ctx context.Context) {
	a.ctx = ctx
	if a.lockout != nil {
		a.lockout.Start(a.onLockoutTriggered)
	}
	if a.auditSvc != nil && a.vaultRepo.IsUnlocked() {
		_ = a.auditSvc.EnforceRetention(a.reqCtx())
	}
}

// Shutdown cleans up all resources when the application closes.
// Order: stop ping → stop lockout → close all sessions → lock vault → close audit log.
//
// The two calls below deliberately do not use reqCtx. Every other handler wants
// to be cancelled when the app context dies; these run *because* it is dying,
// and inheriting it would mean the cleanup is cancelled at exactly the moment
// it is needed - plugin processes left running and the audit retention policy
// silently skipped. They get their own deadline instead, so a wedged plugin
// cannot hold the window open forever either.
func (a *AppAPI) Shutdown() {
	a.StopDebugLogWindow()
	if a.pingMgr != nil {
		a.pingMgr.Stop()
	}
	if a.lockout != nil {
		a.lockout.Stop()
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownCleanupTimeout)
	defer cancelShutdown()

	if a.plugins != nil {
		a.plugins.StopAll(shutdownCtx)
	}
	a.sessions.CloseAll()
	// A local shell is a process this application started; the window closing must take it and
	// everything it spawned with it.
	if a.localTerminals != nil {
		a.localTerminals.CloseAll()
	}

	if a.auditSvc != nil {
		_ = a.auditSvc.EnforceRetention(shutdownCtx)
	}
	a.vaultRepo.Lock()
	if a.auditSvc != nil {
		a.auditSvc.Close()
	}
}

// ReportActivity resets the idle lockout timer. Called from frontend on user interaction.
func (a *AppAPI) ReportActivity() {
	if a.lockout != nil {
		a.lockout.ReportActivity()
	}
}

// ReportMinimized signals that the window was minimized.
func (a *AppAPI) ReportMinimized() {
	if a.lockout != nil {
		a.lockout.ReportMinimized()
	}
}

// ReportRestored signals that the window was restored from minimized.
func (a *AppAPI) ReportRestored() {
	if a.lockout != nil {
		a.lockout.ReportRestored()
	}
}

func (a *AppAPI) onLockoutTriggered() {
	a.lockNow()
}

// lockNow performs the whole lock sequence, and exists so that the two ways a vault gets locked -
// the user asking and the idle timer firing - cannot drift apart. They were separate copies of the
// same four steps: whoever adds a fifth one would have had to remember both, and the copy easiest
// to forget is the timer, which is the one that runs when nobody is watching.
//
// Order matters. Closing the sessions first is what clears the key passphrase cache and tears down
// the live SSH connections while the vault data they were built from still exists; locking first
// would leave both alive with nothing to reconcile them against.
func (a *AppAPI) lockNow() {
	a.sessions.CloseAll()
	// A local shell is a process this application started; the window closing must take it and
	// everything it spawned with it.
	if a.localTerminals != nil {
		a.localTerminals.CloseAll()
	}
	if a.auditSvc != nil {
		a.auditSvc.OnVaultLocked()
	}
	a.vaultRepo.Lock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventVaultLocked, nil)
	}
}

// --- Vault ---

// VaultExists reports whether a vault file already exists, so the frontend can
// choose between the create-master-password and the unlock screen.
func (a *AppAPI) VaultExists() bool {
	return a.vaultRepo.Exists()
}

// afterVaultOpened applies persisted settings to the running managers and runs
// the post-open audit bookkeeping.
//
// Shared by UnlockVault and CreateVault: both leave the vault unlocked and the
// frontend proceeds straight into the app, so a freshly created vault must get
// its lockout timer, ping manager and log level right away rather than on the
// next restart. Keeping it in one place is what stops the two entry points from
// drifting apart.
// restartPing applies ping settings and starts the manager with the callback that publishes
// results to the frontend. Both the unlock path and a settings save need exactly this, and having
// it twice is what let the two drift apart in the first place.
func (a *AppAPI) restartPing(settings domain.PingSettings) {
	if a.pingMgr == nil {
		return
	}
	a.pingMgr.UpdateSettings(settings)
	a.pingMgr.Start(func(results []usecase.PingResult) {
		if a.ctx == nil {
			return
		}
		dtos := make([]PingResultDTO, 0, len(results))
		for _, r := range results {
			dtos = append(dtos, PingResultDTO{ConnectionID: r.ConnectionID, Reachable: r.Reachable, LatencyMs: r.LatencyMs})
		}
		wailsrt.EventsEmit(a.ctx, EventPingUpdated, dtos)
	})
}

func (a *AppAPI) afterVaultOpened() {
	data, err := a.vaultRepo.GetData()
	if err == nil && data.Settings != nil {
		if a.lockout != nil {
			a.lockout.UpdateSettings(data.Settings.Lockout)
		}
		a.restartPing(data.Settings.Ping)
		if a.logLevel != nil {
			a.logLevel.SetLevel(data.Settings.Debug.LogLevel)
		}
		a.SyncDebugLogWindow(data.Settings.Debug.LogWindowEnabled)
		// The language lives in the vault too, so this is the first moment the host can tell the
		// plugins what it is. Plugins start before the vault opens; until now they had only the
		// empty locale the initialize handshake carried.
		if a.localeBroadcast != nil {
			a.localeBroadcast.SetLocale(domain.NormalizeLocaleCode(data.Settings.Language))
		}
	}

	if a.auditSvc != nil {
		a.auditSvc.OnVaultLocked()
		_ = a.auditSvc.EnforceRetention(a.reqCtx())
	}

	// The setting permitting the network call lives in the vault, so this is the first moment the
	// application is allowed to know whether the user wants it.
	a.startUpdateCheck()
}

// LockVault re-locks the vault and clears sensitive data from memory.
func (a *AppAPI) LockVault() {
	a.lockNow()
}

// IsVaultUnlocked returns true if the vault is currently unlocked.
func (a *AppAPI) IsVaultUnlocked() bool {
	return a.vaultRepo.IsUnlocked()
}
