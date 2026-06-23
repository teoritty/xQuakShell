package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
	"ssh-client/internal/infra/persistence"
	"ssh-client/internal/infra/portable"
	infraputty "ssh-client/internal/infra/putty"
	infrasftp "ssh-client/internal/infra/sftp"
	infrassh "ssh-client/internal/infra/ssh"
	presentation "ssh-client/internal/presentation/wails"
	"ssh-client/internal/usecase"
)

// App is the main application struct bound to Wails.
// It delegates all operations to the AppAPI from the presentation layer.
type App struct {
	ctx     context.Context
	api     *presentation.AppAPI
	plugins *pluginRuntime
}

// NewApp creates a new App with all dependencies wired together.
func NewApp() *App {
	paths := portable.Default
	if err := portable.MigrateLegacyLayout(paths); err != nil {
		log.Printf("WARNING: portable data migration failed: %v", err)
	}
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
	localFS := portable.NewLocalFS(portableLayout.DataRoot(), portableLayout.TempDir(), portableRuntime)

	pluginRuntime := newPluginRuntime(vaultDir, pluginRuntimeDeps{
		ConnRepo:        connRepo,
		PasswordRepo:    passwordRepo,
		IdentRepo:       identRepo,
		AuditLog:        auditLogRepo,
		VaultSettings:   usecase.NewPluginVaultSettings(vaultRepo),
		PassphraseCache: sshSession.PassphraseCache,
		ExeDir:          paths.ExeDir(),
	})

	api := presentation.NewAppAPI(
		vaultRepo, connRepo, identRepo, passwordRepo, knownHostsRepo,
		sshDialer, sshSession, newSessionConnectors(),
		auditLogRepo, lockoutMgr, localFS,
		auditlog.NewCommandLineTrackerFactory(),
		auditlog.SanitizerFactory(),
		infraputty.PortAdapter{},
		pluginRuntime.manager, pluginRuntime.inbound, pluginRuntime.viewInbound,
		pluginRuntime.vaultInbound,
	)
	if pluginRuntime.manager != nil {
		pluginRuntime.manager.SetCrashHandler(api.Sessions())
		pluginRuntime.manager.SetSessionOwnershipChecker(api.Sessions())
	}
	if pluginRuntime.viewRelay != nil {
		api.SetPluginViewRelay(pluginRuntime.viewRelay)
	}
	pluginRuntime.setSessionRecoverer(api.Sessions())

	return &App{api: api, plugins: pluginRuntime}
}

func (a *App) grantPluginMultiSessionAccess(pluginID string) error {
	if a.plugins == nil {
		return nil
	}
	return a.plugins.grantMultiSessionAccess(context.Background(), pluginID)
}

func (a *App) grantPluginSecretAccess(pluginID string) error {
	if a.plugins == nil {
		return nil
	}
	return a.plugins.grantSecretAccess(context.Background(), pluginID)
}

func (a *App) pluginAssetHandler() http.Handler {
	if a.plugins == nil {
		return nil
	}
	return a.plugins.assetHandler()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.api.SetContext(ctx)
	a.api.SetPluginVaultGrant(a.grantPluginSecretAccess)
	a.api.SetPluginMultiSessionGrant(a.grantPluginMultiSessionAccess)
	if a.plugins != nil && a.plugins.manager != nil {
		go a.plugins.manager.ActivateStartupPlugins(context.Background())
		a.plugins.manager.SetStateChangeHandler(a.api.EmitPluginStateChanged)
	}
}

func (a *App) shutdown(_ context.Context) {
	if a.plugins != nil {
		a.plugins.shutdown()
	}
	a.api.Shutdown()
}

// --- Wails-bound methods (delegated to AppAPI) ---

func (a *App) UnlockVault(masterPassword string) error {
	return a.api.UnlockVault(masterPassword)
}

func (a *App) LockVault() {
	a.api.LockVault()
}

func (a *App) IsVaultUnlocked() bool {
	return a.api.IsVaultUnlocked()
}

func (a *App) GetFolders() ([]presentation.FolderDTO, error) {
	return a.api.GetFolders()
}

func (a *App) SaveFolder(dto presentation.FolderDTO) (presentation.FolderDTO, error) {
	return a.api.SaveFolder(dto)
}

func (a *App) DeleteFolder(id string) error {
	return a.api.DeleteFolder(id)
}

func (a *App) GetAllConnections() ([]presentation.ConnectionDTO, error) {
	return a.api.GetAllConnections()
}

func (a *App) SaveConnection(dto presentation.ConnectionDTO) (presentation.ConnectionDTO, error) {
	return a.api.SaveConnection(dto)
}

func (a *App) DeleteConnection(id string) error {
	return a.api.DeleteConnection(id)
}

func (a *App) MoveConnections(connectionIDs []string, targetFolderID string) error {
	return a.api.MoveConnections(connectionIDs, targetFolderID)
}

func (a *App) MoveFolder(folderID, targetParentID string) error {
	return a.api.MoveFolder(folderID, targetParentID)
}

func (a *App) ReorderConnections(connectionIDs []string, folderID string) error {
	return a.api.ReorderConnections(connectionIDs, folderID)
}

func (a *App) ReorderFolders(folderIDs []string, parentID string) error {
	return a.api.ReorderFolders(folderIDs, parentID)
}

func (a *App) ImportPassword(password, label string) (string, error) {
	return a.api.ImportPassword(password, label)
}

func (a *App) DeletePassword(id string) error {
	return a.api.DeletePassword(id)
}

func (a *App) ResolveHostKey(sessionID, action, host, authorizedKey string) error {
	return a.api.ResolveHostKey(sessionID, action, host, authorizedKey)
}

func (a *App) SearchAuditLog(query, sessionID, connectionID string, limit, offset int) ([]presentation.AuditEntryDTO, error) {
	return a.api.SearchAuditLog(query, sessionID, connectionID, limit, offset)
}

func (a *App) DeleteAuditEntry(id int64) error {
	return a.api.DeleteAuditEntry(id)
}

func (a *App) ClearAuditLog() error {
	return a.api.ClearAuditLog()
}

func (a *App) GetAuditSessionState() presentation.AuditSessionStateDTO {
	return a.api.GetAuditSessionState()
}

func (a *App) EnableAuditSecretLogging(confirmed bool) error {
	return a.api.EnableAuditSecretLogging(confirmed)
}

func (a *App) DisableAuditSecretLogging() {
	a.api.DisableAuditSecretLogging()
}

func (a *App) ReportActivity() {
	a.api.ReportActivity()
}

func (a *App) ReportMinimized() {
	a.api.ReportMinimized()
}

func (a *App) ReportRestored() {
	a.api.ReportRestored()
}

func (a *App) GetIdentities() ([]presentation.IdentityDTO, error) {
	return a.api.GetIdentities()
}

func (a *App) ImportIdentity(pemBase64, comment string) (string, error) {
	return a.api.ImportIdentity(pemBase64, comment)
}

func (a *App) ImportPuTTYPPK(ppkBase64, passphrase string) (string, error) {
	return a.api.ImportPuTTYPPK(ppkBase64, passphrase)
}

func (a *App) ImportPuTTYReg(regContent string) ([]presentation.PuTTYSessionDTO, error) {
	return a.api.ImportPuTTYReg(regContent)
}

func (a *App) ImportPuTTYRegAsConnections(regContent, folderID string) ([]presentation.ConnectionDTO, error) {
	return a.api.ImportPuTTYRegAsConnections(regContent, folderID)
}

func (a *App) OpenSession(connectionID string) (string, error) {
	return a.api.OpenSession(connectionID)
}

func (a *App) CloseSession(sessionID string) error {
	return a.api.CloseSession(sessionID)
}

func (a *App) GetSessionState(sessionID string) (presentation.SessionDTO, error) {
	return a.api.GetSessionState(sessionID)
}

func (a *App) GetPlatform() string {
	return a.api.GetPlatform()
}

func (a *App) SendTerminalInput(sessionID, data, commandLine string) error {
	return a.api.SendTerminalInput(sessionID, data, commandLine)
}

func (a *App) TerminalResize(sessionID string, cols, rows int) error {
	return a.api.TerminalResize(sessionID, cols, rows)
}

func (a *App) ListPath(sessionID, path string) ([]presentation.RemoteNodeDTO, error) {
	return a.api.ListPath(sessionID, path)
}

func (a *App) RemovePath(sessionID, path string) error {
	return a.api.RemovePath(sessionID, path)
}

func (a *App) MkdirPath(sessionID, parentPath, name string) error {
	return a.api.MkdirPath(sessionID, parentPath, name)
}

func (a *App) CreateFilePath(sessionID, parentPath, name string) error {
	return a.api.CreateFilePath(sessionID, parentPath, name)
}

func (a *App) RenamePath(sessionID, oldPath, newPath string) error {
	return a.api.RenamePath(sessionID, oldPath, newPath)
}

func (a *App) RemoveLocalPath(localPath string) error {
	return a.api.RemoveLocalPath(localPath)
}

func (a *App) MkdirLocalPath(dirPath string) error {
	return a.api.MkdirLocalPath(dirPath)
}

func (a *App) RenameLocalPath(oldPath, newPath string) error {
	return a.api.RenameLocalPath(oldPath, newPath)
}

func (a *App) CreateLocalFile(localPath string) error {
	return a.api.CreateLocalFile(localPath)
}

func (a *App) Upload(sessionID, localPath, remotePath string) error {
	return a.api.Upload(sessionID, localPath, remotePath)
}

func (a *App) Download(sessionID, remotePath, localDir string) error {
	return a.api.Download(sessionID, remotePath, localDir)
}

func (a *App) CancelTransfer(transferID string) {
	a.api.CancelTransfer(transferID)
}

func (a *App) GetKnownHosts() ([]presentation.KnownHostDTO, error) {
	return a.api.GetKnownHosts()
}

func (a *App) AddKnownHost(host, authorizedKey string) error {
	return a.api.AddKnownHost(host, authorizedKey)
}

func (a *App) RemoveKnownHost(host string) error {
	return a.api.RemoveKnownHost(host)
}

func (a *App) SelectLocalFile() (string, error) {
	return a.api.SelectLocalFile()
}

func (a *App) SelectLocalDirectory() (string, error) {
	return a.api.SelectLocalDirectory()
}

func (a *App) GetPingResults() []presentation.PingResultDTO {
	return a.api.GetPingResults()
}

func (a *App) PingConnection(connID string) {
	a.api.PingConnection(connID)
}

func (a *App) ListLocalPath(dirPath string, includeHidden bool) ([]presentation.LocalNodeDTO, error) {
	return a.api.ListLocalPath(dirPath, includeHidden)
}

func (a *App) GetPortableDataRoot() (string, error) {
	return a.api.GetPortableDataRoot()
}

func (a *App) GetUserHomeDir() (string, error) {
	return a.api.GetUserHomeDir()
}

func (a *App) GetTempDir() (string, error) {
	return a.api.GetTempDir()
}

func (a *App) OpenFileWithSystem(localPath, editorPath string) error {
	return a.api.OpenFileWithSystem(localPath, editorPath)
}

func (a *App) StartFileWatch(localPath string) {
	a.api.StartFileWatch(localPath)
}

func (a *App) GetSettings() (presentation.AppSettingsDTO, error) {
	return a.api.GetSettings()
}

func (a *App) SaveSettings(dto presentation.AppSettingsDTO) error {
	return a.api.SaveSettings(dto)
}

func (a *App) ListPlugins() ([]presentation.PluginDTO, error) {
	return a.api.ListPlugins()
}

func (a *App) PingPlugin(pluginID string) (presentation.PluginPingResultDTO, error) {
	return a.api.PingPlugin(pluginID)
}

func (a *App) StartPlugin(pluginID string) error {
	return a.api.StartPlugin(pluginID)
}

func (a *App) SetPluginEnabled(pluginID string, enabled bool) error {
	return a.api.SetPluginEnabled(pluginID, enabled)
}

func (a *App) SelectPluginSourceDir() (string, error) {
	return a.api.SelectPluginSourceDir()
}

func (a *App) PreviewPluginInstall(sourceDir string) (presentation.PluginInstallPreviewDTO, error) {
	return a.api.PreviewPluginInstall(sourceDir)
}

func (a *App) SelectPluginBundleFile() (string, error) {
	return a.api.SelectPluginBundleFile()
}

func (a *App) GetPluginSettings() (presentation.PluginSettingsDTO, error) {
	return a.api.GetPluginSettings()
}

func (a *App) SavePluginSettings(dto presentation.PluginSettingsDTO) error {
	return a.api.SavePluginSettings(dto)
}

func (a *App) GeneratePluginPublisherKeyPair() (presentation.PluginPublisherKeyPairDTO, error) {
	return a.api.GeneratePluginPublisherKeyPair()
}

func (a *App) ValidateTrustedPublisherKey(keyB64 string) error {
	return a.api.ValidateTrustedPublisherKey(keyB64)
}

func (a *App) InstallPlugin(sourceDir string, grantSecretAccess bool, grantMultiSessionAccess bool) (presentation.PluginDTO, error) {
	return a.api.InstallPlugin(sourceDir, grantSecretAccess, grantMultiSessionAccess)
}

func (a *App) GetPluginConnectionProtocols() []presentation.ConnectionProtocolDTO {
	return a.api.GetPluginConnectionProtocols()
}

func (a *App) GetPluginContributions() presentation.PluginContributionsDTO {
	return a.api.GetPluginContributions()
}

func (a *App) ExecutePluginCommand(pluginID, commandID string, args json.RawMessage) (json.RawMessage, error) {
	return a.api.ExecutePluginCommand(pluginID, commandID, args)
}

func (a *App) PreparePluginViewPanel(pluginID, panelID string) (string, error) {
	return a.api.PreparePluginViewPanel(pluginID, panelID)
}

func (a *App) RelayPluginViewMessage(token string, message json.RawMessage) error {
	return a.api.RelayPluginViewMessage(token, message)
}

func (a *App) ReleasePluginViewPanel(token string) {
	a.api.ReleasePluginViewPanel(token)
}
