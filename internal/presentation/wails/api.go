package wails

import (
	"context"
	"sync"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
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
	auditLog            domain.AuditLogRepository
	auditLineTrackers   map[string]*auditlog.CommandLineTracker
	auditLineTrackersMu sync.Mutex
	lockout             domain.LockoutManager
	pingMgr             *usecase.PingManager
	ownerCache          map[string]map[string]string // sessionID -> uid->owner
	groupCache          map[string]map[string]string // sessionID -> gid->group
	ownerCacheMu        sync.Mutex
	transferCancels     map[string]context.CancelFunc // transferID -> cancel
	transferCancelsMu   sync.Mutex
	transferCond        *sync.Cond
	transferActive      int
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
) *AppAPI {
	pingMgr := usecase.NewPingManager(connRepo, domain.DefaultPingSettings())
	api := &AppAPI{
		vaultRepo:         vaultRepo,
		connRepo:          connRepo,
		identRepo:         identRepo,
		passwordRepo:      passwordRepo,
		knownHosts:        knownHosts,
		auditLog:          auditLogRepo,
		auditLineTrackers: make(map[string]*auditlog.CommandLineTracker),
		lockout:           lockoutMgr,
		pingMgr:           pingMgr,
		settingsSvc:       usecase.NewSettingsService(vaultRepo, lockoutMgr, pingMgr),
		ownerCache:        make(map[string]map[string]string),
		groupCache:        make(map[string]map[string]string),
		transferCancels:   make(map[string]context.CancelFunc),
	}
	api.transferCond = sync.NewCond(&sync.Mutex{})

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
		OnStateChange:           api.onSessionStateChange,
		OnStreamReady:           api.onStreamReady,
		PassphraseReq:           api.onPassphraseRequest,
		HostKeyRequest:          api.onHostKeyRequest,
	})

	api.auditSvc = usecase.NewAuditService(auditLogRepo, api.settingsSvc, api.sessions, connRepo)

	return api
}

// SetContext stores the Wails runtime context for event emission and dialogs.
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
	if a.pingMgr != nil {
		a.pingMgr.Stop()
	}
	if a.lockout != nil {
		a.lockout.Stop()
	}
	a.sessions.CloseAll()

	if a.auditSvc != nil {
		_ = a.auditSvc.EnforceRetention(context.Background())
	}
	a.vaultRepo.Lock()
	if a.auditLog != nil {
		a.auditLog.Close()
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
