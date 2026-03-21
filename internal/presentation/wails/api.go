package wails

import (
	"context"
	"os/exec"
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
	vpnProfileRepo      domain.VPNProfileRepository
	sessions            *usecase.SessionManager
	auditLog            domain.AuditLogRepository
	sanitizers          map[string]*auditlog.Sanitizer
	sanitizersMu        sync.Mutex
	auditInputBuffers   map[string]string
	auditInputBuffersMu sync.Mutex
	lockout             domain.LockoutManager
	pingMgr             *usecase.PingManager
	ownerCache          map[string]map[string]string // sessionID -> uid->owner
	groupCache          map[string]map[string]string // sessionID -> gid->group
	ownerCacheMu        sync.Mutex
	transferCancels     map[string]context.CancelFunc // transferID -> cancel
	transferCancelsMu   sync.Mutex
	transferCond        *sync.Cond
	transferActive      int

	rdpProcesses   map[string]*rdpProcess
	rdpProcessesMu sync.Mutex
}

// rdpProcess tracks a running external RDP client process (mstsc / xfreerdp).
type rdpProcess struct {
	cmd  *exec.Cmd
	done chan struct{} // closed when process exits
}

// NewAppAPI creates a new AppAPI with the given dependencies.
func NewAppAPI(
	vaultRepo domain.VaultRepository,
	connRepo domain.ConnectionRepository,
	identRepo domain.IdentityRepository,
	passwordRepo domain.PasswordRepository,
	knownHosts domain.KnownHostsRepository,
	vpnProfileRepo domain.VPNProfileRepository,
	sshFactory domain.SSHClientFactory,
	sshSession usecase.SSHSessionDeps,
	sessionConnectors []domain.SessionConnector,
	auditLogRepo domain.AuditLogRepository,
	lockoutMgr domain.LockoutManager,
) *AppAPI {
	api := &AppAPI{
		vaultRepo:         vaultRepo,
		connRepo:          connRepo,
		identRepo:         identRepo,
		passwordRepo:      passwordRepo,
		knownHosts:        knownHosts,
		vpnProfileRepo:    vpnProfileRepo,
		auditLog:          auditLogRepo,
		sanitizers:        make(map[string]*auditlog.Sanitizer),
		auditInputBuffers: make(map[string]string),
		lockout:           lockoutMgr,
		pingMgr:           usecase.NewPingManager(connRepo, domain.DefaultPingSettings()),
		ownerCache:        make(map[string]map[string]string),
		groupCache:        make(map[string]map[string]string),
		transferCancels:   make(map[string]context.CancelFunc),
		rdpProcesses:      make(map[string]*rdpProcess),
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
		Connectors:              sessionConnectors,
		OnStateChange:           api.onSessionStateChange,
		OnStreamReady:           api.onStreamReady,
		PassphraseReq:           api.onPassphraseRequest,
		HostKeyRequest:          api.onHostKeyRequest,
	})

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
}

// Shutdown cleans up all resources when the application closes.
// Order: stop ping → stop lockout → close all sessions → kill external RDP processes → lock vault → close audit log.
func (a *AppAPI) Shutdown() {
	if a.pingMgr != nil {
		a.pingMgr.Stop()
	}
	if a.lockout != nil {
		a.lockout.Stop()
	}
	a.sessions.CloseAll()

	a.rdpProcessesMu.Lock()
	for sid, proc := range a.rdpProcesses {
		if proc.cmd != nil && proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
		delete(a.rdpProcesses, sid)
	}
	a.rdpProcessesMu.Unlock()

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
					wailsrt.EventsEmit(a.ctx, EventPingUpdated, results)
				}
			})
		}
	}

	return nil
}

// LockVault re-locks the vault and clears sensitive data from memory.
func (a *AppAPI) LockVault() {
	a.sessions.CloseAll()
	a.vaultRepo.Lock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventVaultLocked, nil)
	}
}

// IsVaultUnlocked returns true if the vault is currently unlocked.
func (a *AppAPI) IsVaultUnlocked() bool {
	return a.vaultRepo.IsUnlocked()
}
