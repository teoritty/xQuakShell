package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
	"ssh-client/internal/infra/persistence"
	infrassh "ssh-client/internal/infra/ssh"
	presentation "ssh-client/internal/presentation/wails"
	"ssh-client/internal/usecase"
)

// App is the main application struct bound to Wails.
// It delegates all operations to the AppAPI from the presentation layer.
type App struct {
	ctx context.Context
	api *presentation.AppAPI
}

// NewApp creates a new App with all dependencies wired together.
func NewApp() *App {
	vaultDir := defaultVaultDir()

	vaultRepo := persistence.NewVaultRepo(vaultDir)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	identRepo := persistence.NewIdentityRepo(vaultRepo)
	passwordRepo := persistence.NewPasswordRepo(vaultRepo)
	knownHostsRepo := persistence.NewKnownHostsRepo(vaultRepo)
	vpnProfileRepo := persistence.NewVPNProfileRepo(vaultRepo)
	sshDialer := infrassh.NewDialer()

	auditLogRepo, err := auditlog.NewSQLiteRepo(vaultDir)
	if err != nil {
		log.Printf("WARNING: audit log unavailable: %v", err)
	}

	lockoutMgr := usecase.NewIdleLockoutManager(domain.DefaultLockoutSettings())

	api := presentation.NewAppAPI(vaultRepo, connRepo, identRepo, passwordRepo, knownHostsRepo, vpnProfileRepo, sshDialer, auditLogRepo, lockoutMgr)

	return &App{api: api}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.api.SetContext(ctx)
}

func (a *App) shutdown(_ context.Context) {
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

func (a *App) ImportVPNProfile(configBase64, protocol, label string) (string, error) {
	return a.api.ImportVPNProfile(configBase64, protocol, label)
}

func (a *App) DeleteVPNProfile(id string) error {
	return a.api.DeleteVPNProfile(id)
}

func (a *App) GetVPNProfile(id string) (presentation.VPNProfileDTO, error) {
	return a.api.GetVPNProfile(id)
}

func (a *App) GetVPNProfiles() ([]presentation.VPNProfileDTO, error) {
	return a.api.GetVPNProfiles()
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

func (a *App) RDPStart(sessionID string) (string, error) {
	return a.api.RDPStart(sessionID)
}

func (a *App) RDPStop(sessionID string) error {
	return a.api.RDPStop(sessionID)
}

func (a *App) RDPFocusWindow(sessionID string) error {
	return a.api.RDPFocusWindow(sessionID)
}

func (a *App) GetPlatform() string {
	return a.api.GetPlatform()
}

func (a *App) SendTerminalInput(sessionID, data string) error {
	return a.api.SendTerminalInput(sessionID, data)
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

func (a *App) GetPingResults() []usecase.PingResult {
	return a.api.GetPingResults()
}

func (a *App) PingConnection(connID string) {
	a.api.PingConnection(connID)
}

func (a *App) ListLocalPath(dirPath string, includeHidden bool) ([]presentation.LocalNodeDTO, error) {
	return a.api.ListLocalPath(dirPath, includeHidden)
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

// defaultVaultDir returns the directory for storing the vault file.
// Portable mode: if the executable directory is writable, use it (vault.age next to exe).
// Otherwise use %AppData%\xQuakShell.
func defaultVaultDir() string {
	exe, err := os.Executable()
	if err != nil {
		return fallbackVaultDir()
	}
	exeDir := filepath.Dir(exe)
	testFile := filepath.Join(exeDir, ".xquakshell-writable")
	if err := os.WriteFile(testFile, nil, 0600); err != nil {
		return fallbackVaultDir()
	}
	os.Remove(testFile)
	return exeDir
}

func fallbackVaultDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	return filepath.Join(configDir, "xQuakShell")
}
