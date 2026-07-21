package wails

import (
	"context"
	"log/slog"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/presentation/logwindow"
	"xquakshell/internal/usecase"
)

// AppAPI is the Wails-bound struct that exposes all backend methods to the frontend.
type AppAPI struct {
	ctx                         context.Context
	vaultRepo                   domain.VaultRepository
	vaultSvc                    *usecase.VaultService
	sessions                    *usecase.SessionManager
	settingsSvc                 *usecase.SettingsService
	auditSvc                    *usecase.AuditService
	transferSvc                 *usecase.TransferService
	transferPlanner             *usecase.TransferPlanner
	remoteOpSvc                 *usecase.RemoteOpService
	hostKeys                    *usecase.HostKeyService
	remoteFS                    *usecase.RemoteFSService
	localFS                     *usecase.LocalFSService
	portableData                domain.PortableDataStore
	puttyImport                 *usecase.PuTTYImportService
	lockout                     domain.LockoutManager
	pingMgr                     *usecase.PingManager
	plugins                     *usecase.PluginManager
	viewRelay                   *usecase.PluginViewRelay
	githubRepoService           *usecase.GitHubRepositoryService
	githubPluginService         *usecase.GitHubPluginService
	pluginVaultGrant            func(pluginID string) error
	pluginAuthGrant             func(pluginID string) error
	pluginTunnelGrant           func(pluginID string) error
	pluginMultiSessionGrant     func(pluginID string) error
	pluginArbitraryNetworkGrant func(pluginID string) error
	embedBridge                 *usecase.PluginEmbedBridge
	forwardRules                *usecase.ForwardRuleValidator
	logWindow                   *logwindow.Manager
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
		vaultRepo:    vaultRepo,
		vaultSvc:     vaultSvc,
		portableData: portableData,
		puttyImport:  usecase.NewPuTTYImportService(connRepo, identRepo, puttyImporter),
		lockout:      lockoutMgr,
		pingMgr:      pingMgr,
		plugins:      pluginMgr,
		settingsSvc:  usecase.NewSettingsService(vaultRepo, lockoutMgr, pingMgr),
	}

	smCfg := usecase.SessionManagerConfig{
		ConnRepo:                connRepo,
		VaultRepo:               vaultRepo,
		IdentRepo:               identRepo,
		PasswordRepo:            passwordRepo,
		KnownHosts:              knownHosts,
		SSHFactory:              sshFactory,
		PassphraseCache:         sshSession.PassphraseCache,
		HostKeyCallbackBuilder:  sshSession.HostKeyCallbackBuilder,
		JumpTransportBuilder:    sshSession.JumpTransportBuilder,
		PrivateKeySignerFactory: sshSession.PrivateKeySignerFactory,
		PTYBridgeFactory:        sshSession.PTYBridgeFactory,
		SFTPClientFactory:       sshSession.SFTPClientFactory,
		Connectors:              sessionConnectors,
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
	api.sessions = usecase.NewSessionManager(smCfg)
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
	api.transferSvc = usecase.NewTransferService(api.sessions, api.settingsSvc, hostFS, transferLimiter)
	api.transferPlanner = usecase.NewTransferPlanner(api.sessions, hostFS)
	api.remoteOpSvc = usecase.NewRemoteOpService(api.sessions)
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
	a.logWindow.SyncEnabled(context.Background(), enabled)
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

// SetPluginVaultGrant sets the callback used after install to record secret consent.
func (a *AppAPI) SetPluginVaultGrant(fn func(pluginID string) error) {
	a.pluginVaultGrant = fn
}

// SetPluginAuthGrant sets the callback used after install to record auth provider consent.
func (a *AppAPI) SetPluginAuthGrant(fn func(pluginID string) error) {
	a.pluginAuthGrant = fn
}

// SetPluginTunnelGrant sets the callback used after install to record tunnel provider consent.
func (a *AppAPI) SetPluginTunnelGrant(fn func(pluginID string) error) {
	a.pluginTunnelGrant = fn
}

// SetPluginMultiSessionGrant sets the callback used after install to record multi-session consent.
func (a *AppAPI) SetPluginMultiSessionGrant(fn func(pluginID string) error) {
	a.pluginMultiSessionGrant = fn
}

// SetPluginArbitraryNetworkGrant sets the callback used after install to record arbitrary network consent.
func (a *AppAPI) SetPluginArbitraryNetworkGrant(fn func(pluginID string) error) {
	a.pluginArbitraryNetworkGrant = fn
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
		slog.Debug("embed: OnEmbedReady skipped, nil app ctx", "pluginId", "com.xquakshell.vnc", "sessionId", desc.SessionID)
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
		_ = a.auditSvc.EnforceRetention(context.Background())
	}
}

// Shutdown cleans up all resources when the application closes.
// Order: stop ping → stop lockout → close all sessions → lock vault → close audit log.
func (a *AppAPI) Shutdown() {
	a.StopDebugLogWindow()
	if a.pingMgr != nil {
		a.pingMgr.Stop()
	}
	if a.lockout != nil {
		a.lockout.Stop()
	}
	if a.plugins != nil {
		a.plugins.StopAll(context.Background())
	}
	a.sessions.CloseAll()

	if a.auditSvc != nil {
		_ = a.auditSvc.EnforceRetention(context.Background())
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
	a.sessions.CloseAll()
	if a.auditSvc != nil {
		a.auditSvc.OnVaultLocked()
	}
	a.vaultRepo.Lock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventVaultLocked, nil)
	}
}

// --- Vault ---

// UnlockVault decrypts the vault with the given master password.
// After unlocking, applies persisted settings (e.g. lockout) to the running managers.
func (a *AppAPI) UnlockVault(masterPassword string) error {
	if err := a.vaultRepo.Unlock(context.Background(), masterPassword); err != nil {
		return err
	}

	data, err := a.vaultRepo.GetData()
	if err == nil && data.Settings != nil {
		if a.lockout != nil {
			a.lockout.UpdateSettings(data.Settings.Lockout)
		}
		if a.pingMgr != nil {
			a.pingMgr.UpdateSettings(data.Settings.Ping)
			a.pingMgr.Start(func(results []usecase.PingResult) {
				if a.ctx != nil {
					dtos := make([]PingResultDTO, 0, len(results))
					for _, r := range results {
						dtos = append(dtos, PingResultDTO{ConnectionID: r.ConnectionID, Reachable: r.Reachable, LatencyMs: r.LatencyMs})
					}
					wailsrt.EventsEmit(a.ctx, EventPingUpdated, dtos)
				}
			})
		}
		loghub.SetLevel(loghub.ParseLevel(data.Settings.Debug.LogLevel))
		a.SyncDebugLogWindow(data.Settings.Debug.LogWindowEnabled)
	}

	if a.auditSvc != nil {
		a.auditSvc.OnVaultLocked()
		_ = a.auditSvc.EnforceRetention(context.Background())
	}

	return nil
}

// LockVault re-locks the vault and clears sensitive data from memory.
func (a *AppAPI) LockVault() {
	a.sessions.CloseAll()
	if a.auditSvc != nil {
		a.auditSvc.OnVaultLocked()
	}
	a.vaultRepo.Lock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventVaultLocked, nil)
	}
}

// IsVaultUnlocked returns true if the vault is currently unlocked.
func (a *AppAPI) IsVaultUnlocked() bool {
	return a.vaultRepo.IsUnlocked()
}
