package main

import (
	"log"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/auditlog"
	"xquakshell/internal/infra/host"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/infra/persistence"
	infrapinger "xquakshell/internal/infra/pinger"
	"xquakshell/internal/infra/portable"
	infraputty "xquakshell/internal/infra/putty"
	infrasftp "xquakshell/internal/infra/sftp"
	infrassh "xquakshell/internal/infra/ssh"
	infrasshconfig "xquakshell/internal/infra/sshconfig"
	"xquakshell/internal/pkg/conlimit"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// composeApp wires all dependencies and returns the Wails-bound application.
func composeApp() *App {
	loghub.InstallDefault()
	logStream := loghub.Default()
	log.SetOutput(loghub.NewLineWriter())

	paths := portable.Default
	if err := paths.EnsureDirs(); err != nil {
		log.Printf("WARNING: create portable data dirs failed: %v", err)
	}
	if err := portable.InitRuntime(paths); err != nil {
		log.Printf("WARNING: portable runtime init failed: %v", err)
	}
	if portable.DataRootReadOnly() {
		log.Printf("WARNING: portable data root is read-only; file and plugin writes are disabled")
	}
	vaultDir := paths.VaultDir()

	vaultRepo := persistence.NewVaultRepo(vaultDir)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	identRepo := persistence.NewIdentityRepo(vaultRepo)
	passwordRepo := persistence.NewPasswordRepo(vaultRepo)
	knownHostsRepo := persistence.NewKnownHostsRepo(vaultRepo)
	sshDialer := infrassh.NewDialer()

	auditLogRepo, err := auditlog.NewSQLiteRepo(vaultDir)
	if err != nil {
		log.Printf("WARNING: audit log unavailable: %v", err)
	}

	lockoutMgr := usecase.NewIdleLockoutManager(domain.DefaultLockoutSettings())

	sshSession := usecase.SSHSessionDeps{
		PassphraseCache:         infrassh.NewPassphraseCache(),
		HostKeyCallbackBuilder:  infrassh.NewHostKeyCallbackBuilder(),
		JumpTransportBuilder:    infrassh.NewJumpTransportBuilder(),
		PrivateKeySignerFactory: infrassh.NewPrivateKeySignerFactory(),
		PTYBridgeFactory:        infrassh.NewPTYBridgeFactory(),
		SFTPClientFactory:       infrasftp.NewSFTPClientFactory(),
	}

	portableRuntime := portable.NewRuntimeAdapter()
	portableLayout := portable.NewLayoutAdapter(paths)
	hostFS := host.NewHostFS()
	hostLauncher := host.NewAppLauncher()
	portableData := portable.NewDataStore(portableLayout.DataRoot(), portableLayout.TempDir(), portableRuntime)

	pluginRuntime := newPluginRuntime(vaultDir, portableData, pluginRuntimeDeps{
		ConnRepo:        connRepo,
		PasswordRepo:    passwordRepo,
		IdentRepo:       identRepo,
		AuditLog:        auditLogRepo,
		VaultSettings:   usecase.NewPluginVaultSettings(vaultRepo),
		PassphraseCache: sshSession.PassphraseCache,
		ExeDir:          paths.ExeDir(),
	})

	sshAuth, pluginSessionAudit := wireSSHAuth(pluginRuntime)

	api := presentation.NewAppAPI(
		vaultRepo, connRepo, identRepo, passwordRepo, knownHostsRepo,
		sshDialer, sshSession, newSessionConnectors(),
		auditLogRepo, lockoutMgr, hostFS, hostLauncher, host.IsHiddenLocal, portableData,
		auditlog.NewCommandLineTrackerFactory(),
		auditlog.SanitizerFactory(),
		infraputty.PortAdapter{},
		infrasshconfig.PortAdapter{},
		pluginRuntime.manager, pluginRuntime.inbound, pluginRuntime.viewInbound,
		pluginRuntime.vaultInbound,
		logStream, pluginSessionAudit,
		conlimit.New(domain.DefaultPingSettings().EffectiveMaxConcurrent()),
		conlimit.New(domain.DefaultTransferSettings().MaxConcurrent),
		func() domain.ConcurrencyLimiter { return conlimit.New(64) },
		infrapinger.NewTCPPinger(3*time.Second),
		sshAuth,
		loghub.LevelController{},
	)
	if pluginRuntime.manager != nil && api.Sessions().PluginBridge() != nil {
		bridge := api.Sessions().PluginBridge()
		pluginRuntime.manager.SetCrashHandler(bridge)
		pluginRuntime.manager.SetSessionOwnershipChecker(bridge)
	}
	if pluginRuntime.viewRelay != nil {
		api.SetPluginViewRelay(pluginRuntime.viewRelay)
	}
	api.SetGitHubServices(pluginRuntime.githubRepoService, pluginRuntime.githubPluginService)
	if bridge := api.Sessions().PluginBridge(); bridge != nil {
		pluginRuntime.setSessionRecoverer(bridge)
	}
	pluginRuntime.wireEmbed(api)

	return &App{api: api, plugins: pluginRuntime}
}
