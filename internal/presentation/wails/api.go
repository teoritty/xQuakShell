package wails

import (
	"context"
	"sync"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
	"ssh-client/internal/infra/loghub"
	"ssh-client/internal/presentation/logwindow"
	"ssh-client/internal/usecase"
)

// AppAPI is the Wails-bound struct that exposes all backend methods to the frontend.
type AppAPI struct {
	ctx                 context.Context
	vaultRepo           domain.VaultRepository
	connRepo            domain.ConnectionRepository
	identRepo           domain.IdentityRepository
	passwordRepo        domain.PasswordRepository
	knownHosts          domain.KnownHostsRepository
	sessions            *usecase.SessionManager
	settingsSvc         *usecase.SettingsService
	auditSvc            *usecase.AuditService
	transferSvc         *usecase.TransferService
	hostFS              domain.HostFileSystem
	portableData        domain.PortableDataStore
	puttyImport         *usecase.PuTTYImportService
	lockout             domain.LockoutManager
	pingMgr             *usecase.PingManager
	plugins             *usecase.PluginManager
	pluginFields        *usecase.PluginFieldsService
	viewRelay           *usecase.PluginViewRelay
	githubRepoService   *usecase.GitHubRepositoryService
	githubPluginService *usecase.GitHubPluginService
	pluginVaultGrant              func(pluginID string) error
	pluginMultiSessionGrant       func(pluginID string) error
	pluginArbitraryNetworkGrant   func(pluginID string) error
	embedBridge         *usecase.PluginEmbedBridge
	logWindow           *logwindow.Manager
	ownerCache          map[string]map[string]string // sessionID -> uid->owner
	groupCache          map[string]map[string]string // sessionID -> gid->group
	ownerCacheMu        sync.Mutex
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
	portableData domain.PortableDataStore,
	trackerFactory domain.CommandLineTrackerFactory,
	sanitizerFactory domain.AuditInputSanitizerFactory,
	puttyImporter domain.PuTTYImporter,
	pluginMgr *usecase.PluginManager,
	pluginInbound *usecase.PluginSessionInbound,
	pluginViewInbound *usecase.PluginViewInbound,
	pluginVaultInbound *usecase.PluginVaultInbound,
) *AppAPI {
	pingMgr := usecase.NewPingManager(connRepo, domain.DefaultPingSettings())
	var pluginFieldsSvc *usecase.PluginFieldsService
	var sessionAudit *auditlog.PluginSessionAuditLog
	if pluginMgr != nil {
		pluginFieldsSvc = usecase.NewPluginFieldsService(vaultRepo, pluginMgr.Registry())
		sessionAudit = auditlog.NewPluginSessionAuditLog(512)
		pingMgr.SetProtocolLookup(pluginMgr.Registry())
	}
	api := &AppAPI{
		vaultRepo:         vaultRepo,
		connRepo:          connRepo,
		identRepo:         identRepo,
		passwordRepo:      passwordRepo,
		knownHosts:        knownHosts,
		hostFS:            hostFS,
		portableData:      portableData,
		puttyImport:       usecase.NewPuTTYImportService(connRepo, identRepo, puttyImporter),
		lockout:           lockoutMgr,
		pingMgr:           pingMgr,
		plugins:           pluginMgr,
		pluginFields:      pluginFieldsSvc,
		settingsSvc:       usecase.NewSettingsService(vaultRepo, lockoutMgr, pingMgr),
		ownerCache:        make(map[string]map[string]string),
		groupCache:        make(map[string]map[string]string),
	}

	api.sessions = usecase.NewSessionManager(usecase.SessionManagerConfig{
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
			Audit:   sessionAudit,
		}),
		OnStateChange:           api.onSessionStateChange,
		OnStreamReady:           api.onStreamReady,
		PassphraseReq:           api.onPassphraseRequest,
		HostKeyRequest:          api.onHostKeyRequest,
	})
	if pluginInbound != nil {
		pluginInbound.SetHandler(api.sessions)
	}
	if pluginVaultInbound != nil {
		pluginVaultInbound.SetAuthorizer(api.sessions)
	}
	if pluginViewInbound != nil {
		pluginViewInbound.SetHandler(api)
	}

	api.auditSvc = usecase.NewAuditService(auditLogRepo, api.settingsSvc, api.sessions, connRepo, trackerFactory, sanitizerFactory)
	api.transferSvc = usecase.NewTransferService(api.sessions, api.settingsSvc, hostFS)
	api.logWindow = logwindow.NewManager(loghub.Default(), api.settingsSvc, api.emitDebugLogWindowChanged)

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

// SetPluginMultiSessionGrant sets the callback used after install to record multi-session consent.
func (a *AppAPI) SetPluginMultiSessionGrant(fn func(pluginID string) error) {
	a.pluginMultiSessionGrant = fn
}

// SetPluginArbitraryNetworkGrant sets the callback used after install to record arbitrary network consent.
func (a *AppAPI) SetPluginArbitraryNetworkGrant(fn func(pluginID string) error) {
	a.pluginArbitraryNetworkGrant = fn
}

// SetEmbedBridge wires embed viewport/activity forwarding.
func (a *AppAPI) SetEmbedBridge(bridge *usecase.PluginEmbedBridge) {
	a.embedBridge = bridge
}

// OnEmbedReady emits SessionEmbedReady when a plugin registers an embed surface.
func (a *AppAPI) OnEmbedReady(desc domain.SessionEmbedDescriptor) {
	if a == nil || a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventSessionEmbedReady, SessionEmbedToDTO(desc))
}

// SetGitHubServices wires GitHub repository and plugin services.
func (a *AppAPI) SetGitHubServices(repoSvc *usecase.GitHubRepositoryService, pluginSvc *usecase.GitHubPluginService) {
	a.githubRepoService = repoSvc
	a.githubPluginService = pluginSvc
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
