package wails

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
	"ssh-client/internal/infra/connectors"
	infrassh "ssh-client/internal/infra/ssh"
	infraputty "ssh-client/internal/infra/putty"
	infrasftp "ssh-client/internal/infra/sftp"
	"ssh-client/internal/usecase"
)

// AppAPI is the Wails-bound struct that exposes all backend methods to the frontend.
type AppAPI struct {
	ctx          context.Context
	vaultRepo    domain.VaultRepository
	connRepo       domain.ConnectionRepository
	identRepo      domain.IdentityRepository
	passwordRepo   domain.PasswordRepository
	knownHosts     domain.KnownHostsRepository
	vpnProfileRepo domain.VPNProfileRepository
	sessions       *usecase.SessionManager
	auditLog           domain.AuditLogRepository
	sanitizers         map[string]*auditlog.Sanitizer
	sanitizersMu       sync.Mutex
	auditInputBuffers  map[string]string
	auditInputBuffersMu sync.Mutex
	lockout            domain.LockoutManager
	pingMgr      *usecase.PingManager
	ownerCache        map[string]map[string]string // sessionID -> uid->owner
	groupCache        map[string]map[string]string // sessionID -> gid->group
	ownerCacheMu      sync.Mutex
	transferCancels   map[string]context.CancelFunc // transferID -> cancel
	transferCancelsMu sync.Mutex
	transferCond     *sync.Cond
	transferActive    int

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
	auditLogRepo domain.AuditLogRepository,
	lockoutMgr domain.LockoutManager,
) *AppAPI {
	api := &AppAPI{
		vaultRepo:      vaultRepo,
		connRepo:       connRepo,
		identRepo:      identRepo,
		passwordRepo:   passwordRepo,
		knownHosts:     knownHosts,
		vpnProfileRepo: vpnProfileRepo,
		auditLog:          auditLogRepo,
		sanitizers:        make(map[string]*auditlog.Sanitizer),
		auditInputBuffers: make(map[string]string),
		lockout:           lockoutMgr,
		pingMgr:      usecase.NewPingManager(connRepo, domain.DefaultPingSettings()),
		ownerCache:   make(map[string]map[string]string),
		groupCache:   make(map[string]map[string]string),
		transferCancels: make(map[string]context.CancelFunc),
		rdpProcesses:   make(map[string]*rdpProcess),
	}
	api.transferCond = sync.NewCond(&sync.Mutex{})

	api.sessions = usecase.NewSessionManager(usecase.SessionManagerConfig{
		ConnRepo:       connRepo,
		VaultRepo:      vaultRepo,
		IdentRepo:      identRepo,
		PasswordRepo:   passwordRepo,
		KnownHosts:     knownHosts,
		SSHFactory:     sshFactory,
		Connectors: []domain.SessionConnector{
			connectors.NewTelnetConnector(),
			connectors.NewSerialConnector(),
			connectors.NewRDPConnector(),
			connectors.NewHTTPConnector(),
		},
		OnStateChange:  api.onSessionStateChange,
		OnStreamReady:  api.onStreamReady,
		PassphraseReq:  api.onPassphraseRequest,
		HostKeyRequest: api.onHostKeyRequest,
	})

	return api
}

// SetContext stores the Wails runtime context for event emission and dialogs.
// Also starts the lockout manager if configured.
func (a *AppAPI) SetContext(ctx context.Context) {
	a.ctx = ctx
	if a.lockout != nil {
		a.lockout.Start(a.onLockoutTriggered)
	}
}

// Shutdown cleans up all resources when the application closes.
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

// --- Folders ---

// GetFolders returns all folders.
func (a *AppAPI) GetFolders() ([]FolderDTO, error) {
	fs, err := a.connRepo.GetAllFolders(context.Background())
	if err != nil {
		return nil, err
	}
	return FoldersToDTO(fs), nil
}

// SaveFolder creates or updates a folder.
func (a *AppAPI) SaveFolder(dto FolderDTO) (FolderDTO, error) {
	f := DTOToFolder(dto)
	if err := a.connRepo.SaveFolder(context.Background(), &f); err != nil {
		return FolderDTO{}, err
	}
	return FolderToDTO(f), nil
}

// DeleteFolder removes a folder (connections move to root).
func (a *AppAPI) DeleteFolder(id string) error {
	return a.connRepo.DeleteFolder(context.Background(), id)
}

// --- Connections ---

// GetAllConnections returns all connections.
func (a *AppAPI) GetAllConnections() ([]ConnectionDTO, error) {
	cs, err := a.connRepo.GetAllConnections(context.Background())
	if err != nil {
		return nil, err
	}
	return ConnectionsToDTO(cs), nil
}

// SaveConnection creates or updates a connection.
func (a *AppAPI) SaveConnection(dto ConnectionDTO) (ConnectionDTO, error) {
	c := DTOToConnection(dto)
	if err := a.connRepo.Save(context.Background(), &c); err != nil {
		return ConnectionDTO{}, err
	}
	if a.pingMgr != nil {
		if h := c.EffectiveHost(); h != "" && c.EffectivePort() > 0 {
			a.pingMgr.PingSingle(c.ID, h, c.EffectivePort())
		}
	}
	return ConnectionToDTO(c), nil
}

// DeleteConnection removes a connection by ID.
func (a *AppAPI) DeleteConnection(id string) error {
	return a.connRepo.Delete(context.Background(), id)
}

// MoveConnections moves connections to a target folder.
func (a *AppAPI) MoveConnections(connectionIDs []string, targetFolderID string) error {
	return a.connRepo.MoveToFolder(context.Background(), connectionIDs, targetFolderID)
}

// MoveFolder changes a folder's parent.
func (a *AppAPI) MoveFolder(folderID, targetParentID string) error {
	return a.connRepo.MoveFolder(context.Background(), folderID, targetParentID)
}

// ReorderConnections updates the order of connections within a folder.
func (a *AppAPI) ReorderConnections(connectionIDs []string, folderID string) error {
	return a.connRepo.ReorderConnections(context.Background(), connectionIDs, folderID)
}

// ReorderFolders updates the order of folders under a parent.
func (a *AppAPI) ReorderFolders(folderIDs []string, parentID string) error {
	return a.connRepo.ReorderFolders(context.Background(), folderIDs, parentID)
}

// --- Passwords ---

// ImportPassword stores a password in the vault and returns its ID.
func (a *AppAPI) ImportPassword(password, label string) (string, error) {
	return a.passwordRepo.Import(context.Background(), []byte(password), label)
}

// DeletePassword removes a password from the vault.
func (a *AppAPI) DeletePassword(id string) error {
	return a.passwordRepo.Delete(context.Background(), id)
}

// --- VPN Profiles ---

// VPNProfileDTO is the UI-facing representation of a VPN profile.
type VPNProfileDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
}

// ImportVPNProfile stores a VPN config in the vault and returns the profile ID.
func (a *AppAPI) ImportVPNProfile(configBase64, protocol, label string) (string, error) {
	configBlob, err := base64.StdEncoding.DecodeString(configBase64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 config: %w", err)
	}
	prof := &domain.VPNProfile{
		Label:      label,
		Protocol:   domain.VPNProtocol(protocol),
		ConfigBlob: configBlob,
	}
	if err := a.vpnProfileRepo.Save(context.Background(), prof); err != nil {
		return "", err
	}
	return prof.ID, nil
}

// DeleteVPNProfile removes a VPN profile from the vault.
func (a *AppAPI) DeleteVPNProfile(id string) error {
	return a.vpnProfileRepo.Delete(context.Background(), id)
}

// GetVPNProfile returns a VPN profile by ID.
func (a *AppAPI) GetVPNProfile(id string) (VPNProfileDTO, error) {
	prof, err := a.vpnProfileRepo.Get(context.Background(), id)
	if err != nil {
		return VPNProfileDTO{}, err
	}
	return VPNProfileDTO{ID: prof.ID, Label: prof.Label, Protocol: string(prof.Protocol)}, nil
}

// GetVPNProfiles returns all VPN profiles.
func (a *AppAPI) GetVPNProfiles() ([]VPNProfileDTO, error) {
	profiles, err := a.vpnProfileRepo.GetAll(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]VPNProfileDTO, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, VPNProfileDTO{ID: p.ID, Label: p.Label, Protocol: string(p.Protocol)})
	}
	return result, nil
}

// --- Identities ---

// GetIdentities returns metadata for all SSH identities.
func (a *AppAPI) GetIdentities() ([]IdentityDTO, error) {
	ids, err := a.identRepo.GetAll(context.Background())
	if err != nil {
		return nil, err
	}
	return IdentitiesToDTO(ids), nil
}

// ImportIdentity imports a PEM private key (base64-encoded) into the vault.
// Returns the new identity ID.
func (a *AppAPI) ImportIdentity(pemBase64, comment string) (string, error) {
	pemData, err := base64.StdEncoding.DecodeString(pemBase64)
	if err != nil {
		return "", fmt.Errorf("decode pem base64: %w", err)
	}
	identity, err := a.identRepo.Import(context.Background(), pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// ImportPuTTYPPK imports a PuTTY .ppk file (base64-encoded content) into the vault as an identity.
// passphrase is required if the PPK is encrypted.
func (a *AppAPI) ImportPuTTYPPK(ppkBase64, passphrase string) (string, error) {
	ppkData, err := base64.StdEncoding.DecodeString(ppkBase64)
	if err != nil {
		return "", fmt.Errorf("decode ppk base64: %w", err)
	}
	pemData, comment, err := infraputty.PPKToPEM(ppkData, passphrase)
	if err != nil {
		return "", err
	}
	if comment == "" {
		comment = "PuTTY import"
	}
	identity, err := a.identRepo.Import(context.Background(), pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// PuTTYSessionDTO is a preview item for REG import.
type PuTTYSessionDTO struct {
	Name     string `json:"name"`
	HostName string `json:"hostName"`
	Port     int    `json:"port"`
	UserName string `json:"userName"`
}

// ImportPuTTYReg parses a PuTTY .reg file and returns session previews.
func (a *AppAPI) ImportPuTTYReg(regContent string) ([]PuTTYSessionDTO, error) {
	sessions, err := infraputty.ParsePuTTYReg(regContent)
	if err != nil {
		return nil, err
	}
	result := make([]PuTTYSessionDTO, len(sessions))
	for i, s := range sessions {
		result[i] = PuTTYSessionDTO{
			Name:     s.Name,
			HostName: s.HostName,
			Port:     s.Port,
			UserName: s.UserName,
		}
	}
	return result, nil
}

// ImportPuTTYRegAsConnections parses a PuTTY .reg file and creates connections in the given folder.
func (a *AppAPI) ImportPuTTYRegAsConnections(regContent, folderID string) ([]ConnectionDTO, error) {
	sessions, err := infraputty.ParsePuTTYReg(regContent)
	if err != nil {
		return nil, err
	}
	var result []ConnectionDTO
	for i, s := range sessions {
		if s.HostName == "" {
			continue
		}
		conn := s.ToConnection(folderID, i)
		conn.ID = ""
		if err := a.connRepo.Save(context.Background(), &conn); err != nil {
			return result, fmt.Errorf("save session %s: %w", s.Name, err)
		}
		result = append(result, ConnectionToDTO(conn))
	}
	return result, nil
}

// --- Sessions ---

// OpenSession starts a new SSH session for the given connection.
// Returns the session ID; the connection process is async.
func (a *AppAPI) OpenSession(connectionID string) (string, error) {
	sessionID, err := a.sessions.OpenSession(connectionID)
	if err != nil {
		return "", err
	}

	go a.initSessionPTYAndSFTP(sessionID)

	return sessionID, nil
}

// CloseSession terminates a session by its ID.
func (a *AppAPI) CloseSession(sessionID string) error {
	if info, err := a.sessions.GetState(sessionID); err == nil && info.Protocol == domain.ProtocolRDP {
		_ = a.RDPStop(sessionID)
	}
	err := a.sessions.CloseSession(sessionID)
	if err != nil {
		return err
	}
	a.auditInputBuffersMu.Lock()
	delete(a.auditInputBuffers, sessionID)
	a.auditInputBuffersMu.Unlock()
	a.ownerCacheMu.Lock()
	delete(a.ownerCache, sessionID)
	delete(a.groupCache, sessionID)
	a.ownerCacheMu.Unlock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSessionClosed, map[string]string{"sessionId": sessionID})
	}
	go func() {
		runtime.GC()
	}()
	return nil
}

// GetSessionState returns the current state of a session.
func (a *AppAPI) GetSessionState(sessionID string) (SessionDTO, error) {
	info, err := a.sessions.GetState(sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	return SessionToDTO(info), nil
}

// RDPStart launches a native external RDP client (mstsc on Windows, xfreerdp on Linux)
// and tracks the process by sessionID. If a process is already running for this session,
// it focuses the existing window instead of launching a duplicate.
// Returns "native" to indicate the session is opened in an external window.
func (a *AppAPI) RDPStart(sessionID string) (string, error) {
	info, err := a.sessions.GetState(sessionID)
	if err != nil {
		return "", err
	}
	if info.Protocol != domain.ProtocolRDP {
		return "", fmt.Errorf("session is not RDP")
	}

	a.rdpProcessesMu.Lock()
	if existing, ok := a.rdpProcesses[sessionID]; ok {
		select {
		case <-existing.done:
			delete(a.rdpProcesses, sessionID)
		default:
			a.rdpProcessesMu.Unlock()
			_ = connectors.FocusWindowByPID(existing.cmd.Process.Pid)
			return "native", nil
		}
	}
	a.rdpProcessesMu.Unlock()

	conn, err := a.connRepo.GetByID(context.Background(), info.ConnectionID)
	if err != nil {
		return "", err
	}
	if conn.RDPConfig == nil || conn.RDPConfig.Host == "" {
		return "", fmt.Errorf("rdp host not configured")
	}

	var password string
	if conn.RDPConfig.PasswordID != "" {
		if pw, e := a.passwordRepo.Get(context.Background(), conn.RDPConfig.PasswordID); e == nil {
			password = string(pw)
		}
	}

	cmd, err := connectors.StartExternalRDPProcess(conn, password)
	if err != nil {
		return "", err
	}

	proc := &rdpProcess{cmd: cmd, done: make(chan struct{})}
	a.rdpProcessesMu.Lock()
	a.rdpProcesses[sessionID] = proc
	a.rdpProcessesMu.Unlock()

	go func() {
		_ = cmd.Wait()
		close(proc.done)
		a.rdpProcessesMu.Lock()
		if a.rdpProcesses[sessionID] == proc {
			delete(a.rdpProcesses, sessionID)
		}
		a.rdpProcessesMu.Unlock()
	}()

	return "native", nil
}

// RDPStop terminates the tracked external RDP client for the given session.
func (a *AppAPI) RDPStop(sessionID string) error {
	a.rdpProcessesMu.Lock()
	proc, ok := a.rdpProcesses[sessionID]
	if ok {
		delete(a.rdpProcesses, sessionID)
	}
	a.rdpProcessesMu.Unlock()

	if ok && proc.cmd != nil && proc.cmd.Process != nil {
		select {
		case <-proc.done:
		default:
			_ = proc.cmd.Process.Kill()
		}
	}
	return nil
}

// RDPFocusWindow brings the external RDP client window to the foreground for the given session.
func (a *AppAPI) RDPFocusWindow(sessionID string) error {
	a.rdpProcessesMu.Lock()
	proc, ok := a.rdpProcesses[sessionID]
	a.rdpProcessesMu.Unlock()

	if !ok {
		return fmt.Errorf("no RDP process running for session %s", sessionID)
	}

	select {
	case <-proc.done:
		return fmt.Errorf("RDP process has exited for session %s", sessionID)
	default:
	}

	return connectors.FocusWindowByPID(proc.cmd.Process.Pid)
}

// GetPlatform returns the current OS: "windows", "linux", "darwin".
func (a *AppAPI) GetPlatform() string {
	return runtime.GOOS
}

// --- Terminal ---

// SendTerminalInput sends keyboard input to a session's PTY and logs to audit.
func (a *AppAPI) SendTerminalInput(sessionID, data string) error {
	bridge, err := a.sessions.GetPTYBridge(sessionID)
	if err != nil {
		return err
	}
	if err := bridge.Write([]byte(data)); err != nil {
		return err
	}

	if a.auditLog != nil {
		go a.bufferAndAppendAuditInput(sessionID, data)
	}
	return nil
}

// bufferAndAppendAuditInput buffers terminal input and appends to audit only on newline (\n or \r).
// Escape sequences (e.g. arrow keys) are not buffered.
func (a *AppAPI) bufferAndAppendAuditInput(sessionID, data string) {
	// Skip escape sequences (e.g. \x1b[A for arrow up) - don't log control sequences
	if len(data) > 0 && data[0] == '\x1b' && (len(data) == 1 || (len(data) > 1 && data[1] != '\n' && data[1] != '\r')) {
		return
	}

	a.auditInputBuffersMu.Lock()
	buf := a.auditInputBuffers[sessionID] + data
	a.auditInputBuffers[sessionID] = ""
	a.auditInputBuffersMu.Unlock()

	// Split by newlines; last segment may be incomplete
	lines := splitLines(buf)
	if len(lines) == 0 {
		return
	}

	// All but the last are complete lines to log
	for i := 0; i < len(lines)-1; i++ {
		trimmed := trimLineEnding(lines[i])
		if trimmed != "" {
			a.appendAuditEntry(sessionID, trimmed)
		}
	}

	// Last segment: if it ends with newline, it's complete; otherwise put back in buffer
	last := lines[len(lines)-1]
	if len(last) > 0 && (last[len(last)-1] == '\n' || last[len(last)-1] == '\r') {
		trimmed := trimLineEnding(last)
		if trimmed != "" {
			a.appendAuditEntry(sessionID, trimmed)
		}
	} else {
		a.auditInputBuffersMu.Lock()
		a.auditInputBuffers[sessionID] = last
		a.auditInputBuffersMu.Unlock()
	}
}

func splitLines(s string) []string {
	var lines []string
	var cur []rune
	for _, r := range s {
		if r == '\n' || r == '\r' {
			cur = append(cur, r)
			lines = append(lines, string(cur))
			cur = cur[:0]
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return lines
}

func trimLineEnding(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func (a *AppAPI) appendAuditEntry(sessionID, input string) {
	sanitizer := a.getSanitizer(sessionID)
	sanitized, redacted := sanitizer.SanitizeInput(input)

	info, err := a.sessions.GetState(sessionID)
	if err != nil {
		return
	}

	conn, _ := a.connRepo.GetByID(context.Background(), info.ConnectionID)
	username := ""
	if conn != nil {
		username = conn.EffectiveUsername()
	}

	entry := domain.AuditEntry{
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		ConnectionID: info.ConnectionID,
		Username:     username,
		Input:        sanitized,
		Redacted:     redacted,
	}
	_ = a.auditLog.Append(context.Background(), entry)
}

func (a *AppAPI) getSanitizer(sessionID string) *auditlog.Sanitizer {
	a.sanitizersMu.Lock()
	defer a.sanitizersMu.Unlock()
	s, ok := a.sanitizers[sessionID]
	if !ok {
		s = auditlog.NewSanitizer()
		a.sanitizers[sessionID] = s
	}
	return s
}

// TerminalResize changes the PTY window size for a session.
func (a *AppAPI) TerminalResize(sessionID string, cols, rows int) error {
	bridge, err := a.sessions.GetPTYBridge(sessionID)
	if err != nil {
		return err
	}
	return bridge.Resize(uint32(cols), uint32(rows))
}

// --- SFTP ---

// runSSHCommand executes a command on the remote host via SSH.
func (a *AppAPI) runSSHCommand(sessionID, cmd string) (string, error) {
	sshClient, err := a.sessions.GetSSHClient(sessionID)
	if err != nil {
		return "", err
	}
	session, err := sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isNumeric returns true if s contains only digits.
func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// resolveOwner resolves UID to username via getent passwd; uses cache.
func (a *AppAPI) resolveOwner(sessionID, uid string) string {
	if !isNumeric(uid) {
		return uid
	}
	a.ownerCacheMu.Lock()
	if a.ownerCache[sessionID] == nil {
		a.ownerCache[sessionID] = make(map[string]string)
	}
	if name, ok := a.ownerCache[sessionID][uid]; ok {
		a.ownerCacheMu.Unlock()
		return name
	}
	a.ownerCacheMu.Unlock()
	out, err := a.runSSHCommand(sessionID, "getent passwd "+uid)
	if err != nil {
		return uid
	}
	// getent passwd returns "username:x:uid:gid:..."
	fields := strings.SplitN(out, ":", 2)
	name := uid
	if len(fields) >= 1 && fields[0] != "" {
		name = fields[0]
	}
	a.ownerCacheMu.Lock()
	a.ownerCache[sessionID][uid] = name
	a.ownerCacheMu.Unlock()
	return name
}

// resolveGroup resolves GID to group name via getent group; uses cache.
func (a *AppAPI) resolveGroup(sessionID, gid string) string {
	if !isNumeric(gid) {
		return gid
	}
	a.ownerCacheMu.Lock()
	if a.groupCache[sessionID] == nil {
		a.groupCache[sessionID] = make(map[string]string)
	}
	if name, ok := a.groupCache[sessionID][gid]; ok {
		a.ownerCacheMu.Unlock()
		return name
	}
	a.ownerCacheMu.Unlock()
	out, err := a.runSSHCommand(sessionID, "getent group "+gid)
	if err != nil {
		return gid
	}
	fields := strings.SplitN(out, ":", 2)
	name := gid
	if len(fields) >= 1 && fields[0] != "" {
		name = fields[0]
	}
	a.ownerCacheMu.Lock()
	a.groupCache[sessionID][gid] = name
	a.ownerCacheMu.Unlock()
	return name
}

// ListPath lists the contents of a remote directory.
func (a *AppAPI) ListPath(sessionID, path string) ([]RemoteNodeDTO, error) {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return nil, err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return nil, err
	}
	nodes, err := fs.List(ctx, path)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].Owner != "" {
			nodes[i].Owner = a.resolveOwner(sessionID, nodes[i].Owner)
		}
		if nodes[i].Group != "" {
			nodes[i].Group = a.resolveGroup(sessionID, nodes[i].Group)
		}
	}
	return RemoteNodesToDTO(nodes), nil
}

// RemovePath deletes a remote file or directory (recursively for directories).
func (a *AppAPI) RemovePath(sessionID, path string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.RemoveAll(ctx, path)
}

// MkdirPath creates a remote directory (and parents if needed).
func (a *AppAPI) MkdirPath(sessionID, parentPath, name string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	fullPath := path.Join(parentPath, name)
	return fs.Mkdir(ctx, fullPath)
}

// CreateFilePath creates an empty remote file.
func (a *AppAPI) CreateFilePath(sessionID, parentPath, name string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	fullPath := path.Join(parentPath, name)
	return fs.CreateFile(ctx, fullPath)
}

// RenamePath renames a remote file or directory.
func (a *AppAPI) RenamePath(sessionID, oldPath, newPath string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Rename(ctx, oldPath, newPath)
}

// --- Ping ---

// GetPingResults returns the current ping results for all connections.
func (a *AppAPI) GetPingResults() []usecase.PingResult {
	if a.pingMgr == nil {
		return nil
	}
	return a.pingMgr.GetResults()
}

// PingConnection pings a single connection immediately.
func (a *AppAPI) PingConnection(connID string) {
	if a.pingMgr == nil {
		return
	}
	conn, err := a.connRepo.GetByID(context.Background(), connID)
	if err != nil {
		return
	}
	host := conn.EffectiveHost()
	port := conn.EffectivePort()
	if host == "" || port <= 0 {
		return
	}
	go a.pingMgr.PingSingle(connID, host, port)
}

// LocalNodeDTO represents a local file or directory entry.
type LocalNodeDTO struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
}

// ListLocalPath returns directory entries for a local path.
// includeHidden when false filters out hidden files (name starts with . on Unix, HIDDEN attribute on Windows).
func (a *AppAPI) ListLocalPath(dirPath string, includeHidden bool) ([]LocalNodeDTO, error) {
	if dirPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dirPath = home
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var result []LocalNodeDTO
	for _, e := range entries {
		if !includeHidden && isHiddenLocal(filepath.Join(dirPath, e.Name()), e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dirPath, e.Name())
		dto := LocalNodeDTO{
			Name:    e.Name(),
			Path:    fullPath,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Mode:    info.Mode().String(),
		}
		dto.Owner = getLocalFileOwner(info, fullPath)
		result = append(result, dto)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// RemoveLocalPath deletes a local file or directory (recursively for directories).
func (a *AppAPI) RemoveLocalPath(localPath string) error {
	return os.RemoveAll(localPath)
}

// MkdirLocalPath creates a local directory (and parents if needed).
func (a *AppAPI) MkdirLocalPath(dirPath string) error {
	return os.MkdirAll(dirPath, 0o755)
}

// RenameLocalPath renames a local file or directory.
func (a *AppAPI) RenameLocalPath(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// CreateLocalFile creates an empty local file.
func (a *AppAPI) CreateLocalFile(localPath string) error {
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	return f.Close()
}

// GetUserHomeDir returns the current user's home directory.
func (a *AppAPI) GetUserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// GetTempDir returns the system temp directory.
func (a *AppAPI) GetTempDir() (string, error) {
	return os.TempDir(), nil
}

// StartFileWatch watches a local file for changes and emits FileEdited when mtime changes.
// Polls every 500ms; stops after first change or after 1 hour.
func (a *AppAPI) StartFileWatch(localPath string) {
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	initialMod := info.ModTime()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(time.Hour)
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				info, err := os.Stat(abs)
				if err != nil {
					return
				}
				if info.ModTime().After(initialMod) {
					if a.ctx != nil {
						wailsrt.EventsEmit(a.ctx, EventFileEdited, map[string]string{"localPath": localPath})
					}
					return
				}
			}
		}
	}()
}

// OpenFileWithSystem opens a local file with the system's default application or the specified editor.
// If editorPath is non-empty, runs editorPath with localPath as argument; otherwise uses system default.
func (a *AppAPI) OpenFileWithSystem(localPath, editorPath string) error {
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	if editorPath != "" {
		editorPath = strings.TrimSpace(editorPath)
		if editorPath != "" {
			return exec.Command(editorPath, abs).Start()
		}
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/C", "start", "", abs).Start()
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}

// acquireTransferSlot blocks until a transfer slot is available. Call releaseTransferSlot when done.
func (a *AppAPI) acquireTransferSlot(ctx context.Context) error {
	limit := 4
	if data, err := a.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.MaxConcurrent > 0 {
		limit = data.Settings.Transfer.MaxConcurrent
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			a.transferCond.Broadcast()
		case <-done:
		}
	}()
	a.transferCond.L.Lock()
	for a.transferActive >= limit {
		a.transferCond.Wait()
		if ctx.Err() != nil {
			a.transferCond.L.Unlock()
			return ctx.Err()
		}
	}
	a.transferActive++
	a.transferCond.L.Unlock()
	return nil
}

func (a *AppAPI) releaseTransferSlot() {
	a.transferCond.L.Lock()
	a.transferActive--
	a.transferCond.Signal()
	a.transferCond.L.Unlock()
}

// Upload copies a local file or directory to the remote path (recursive for directories).
func (a *AppAPI) Upload(sessionID, localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return a.uploadRecursive(sessionID, localPath, remotePath)
	}
	return a.uploadFile(sessionID, localPath, remotePath)
}

func (a *AppAPI) uploadFile(sessionID, localPath, remotePath string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localPath))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
				ID: transferID, SessionID: sessionID, Direction: "upload",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.Upload(ctx, localPath, remotePath, progress)
	}()
	err = <-doneCh

	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
			_ = fs.Remove(context.Background(), remotePath) // try to remove partial remote file
		} else {
			state = "failed"
		}
	}
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "upload",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

// Download copies a remote file or directory to the local path (recursive for directories).
func (a *AppAPI) Download(sessionID, remotePath, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	_, listErr := fs.List(ctx, remotePath)
	if listErr == nil {
		localTarget := filepath.Join(localDir, filepath.Base(remotePath))
		if err := os.MkdirAll(localTarget, 0755); err != nil {
			return err
		}
		return a.downloadRecursive(sessionID, remotePath, localTarget)
	}
	return a.downloadFile(sessionID, remotePath, localDir)
}

func (a *AppAPI) downloadRecursive(sessionID, remoteDir, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remoteDir))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
				ID: transferID, SessionID: sessionID, Direction: "download",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.DownloadRecursive(ctx, remoteDir, localDir, progress)
	}()
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
		} else {
			state = "failed"
		}
	}
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "download",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (a *AppAPI) downloadFile(sessionID, remotePath, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	localPath := filepath.Join(localDir, filepath.Base(remotePath))
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remotePath))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
				ID: transferID, SessionID: sessionID, Direction: "download",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.Download(ctx, remotePath, localPath, progress)
	}()
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
			_ = os.Remove(localPath) // remove partial local file
		} else {
			state = "failed"
		}
	}
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "download",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (a *AppAPI) uploadRecursive(sessionID, localDir, remoteDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localDir))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
				ID: transferID, SessionID: sessionID, Direction: "upload",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.UploadRecursive(ctx, localDir, remoteDir, progress)
	}()
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
		} else {
			state = "failed"
		}
	}
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "upload",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

// CancelTransfer cancels an active transfer by ID.
func (a *AppAPI) CancelTransfer(transferID string) {
	a.transferCancelsMu.Lock()
	cancel, ok := a.transferCancels[transferID]
	delete(a.transferCancels, transferID)
	a.transferCancelsMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

// --- Known Hosts ---

// GetKnownHosts returns all known host entries.
func (a *AppAPI) GetKnownHosts() ([]KnownHostDTO, error) {
	entries, err := a.knownHosts.List()
	if err != nil {
		return nil, err
	}
	return KnownHostsToDTO(entries), nil
}

// AddKnownHost adds a known host entry from an authorized_key formatted string.
func (a *AppAPI) AddKnownHost(host, authorizedKey string) error {
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return fmt.Errorf("parse authorized key: %w", err)
	}
	return a.knownHosts.Add(context.Background(), host, key)
}

// RemoveKnownHost removes a known host entry by host pattern.
func (a *AppAPI) RemoveKnownHost(host string) error {
	return a.knownHosts.Remove(context.Background(), host)
}

// --- File Dialogs ---

// SelectLocalFile opens a native file picker and returns the selected file path.
func (a *AppAPI) SelectLocalFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenFileDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select File",
	})
}

// SelectLocalDirectory opens a native directory picker.
func (a *AppAPI) SelectLocalDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select Directory",
	})
}

// --- Settings ---

// AppSettingsDTO is the UI-facing representation of application settings.
type AppSettingsDTO struct {
	LockoutEnabled           bool   `json:"lockoutEnabled"`
	LockoutIdleMinutes       int    `json:"lockoutIdleMinutes"`
	LockOnMinimize           bool   `json:"lockOnMinimize"`
	TerminalFontFamily       string `json:"terminalFontFamily"`
	TerminalFontSize         int    `json:"terminalFontSize"`
	TerminalFontColor        string `json:"terminalFontColor"`
	Theme                    string `json:"theme"`
	PingEnabled              bool   `json:"pingEnabled"`
	PingMode                 string `json:"pingMode"`           // "interval" or "on_change"
	PingIntervalSeconds      int    `json:"pingIntervalSeconds"` // used when mode=interval
	PingIntervalMin          int    `json:"pingIntervalMin"`     // deprecated, for migration
	ExternalEditorPath       string `json:"externalEditorPath"`
	TransferSpeedLimitKbps   int    `json:"transferSpeedLimitKbps"`
	ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	MaxConcurrentTransfers   int    `json:"maxConcurrentTransfers"`
	SessionHotkeyCreate      string `json:"sessionHotkeyCreate"`
	SessionHotkeyNext        string `json:"sessionHotkeyNext"`
	SessionHotkeyPrev        string `json:"sessionHotkeyPrev"`
	SessionHotkeyClose       string `json:"sessionHotkeyClose"`
}

// GetSettings returns the current application settings.
func (a *AppAPI) GetSettings() (AppSettingsDTO, error) {
	data, err := a.vaultRepo.GetData()
	if err != nil {
		return AppSettingsDTO{}, err
	}
	s := data.Settings
	if s == nil {
		lockout := domain.DefaultLockoutSettings()
		terminal := domain.DefaultTerminalSettings()
		ping := domain.DefaultPingSettings()
		transfer := domain.DefaultTransferSettings()
		hotkeys := domain.DefaultSessionHotkeysSettings()
		return AppSettingsDTO{
			LockoutEnabled:           lockout.Enabled,
			LockoutIdleMinutes:       int(lockout.IdleTimeout.Minutes()),
			LockOnMinimize:           lockout.LockOnMinimize,
			TerminalFontFamily:       terminal.FontFamily,
			TerminalFontSize:         terminal.FontSize,
			TerminalFontColor:        terminal.FontColor,
			Theme:                    "dark",
			PingEnabled:              ping.Enabled,
			PingMode:                 ping.Mode,
			PingIntervalSeconds:      ping.EffectiveIntervalSeconds(),
			ExternalEditorPath:       "",
			TransferSpeedLimitKbps:   transfer.SpeedLimitKbps,
			ConnectionTimeoutSeconds: transfer.ConnectionTimeoutSec,
			MaxConcurrentTransfers:   transfer.MaxConcurrent,
			SessionHotkeyCreate:      hotkeys.Create,
			SessionHotkeyNext:        hotkeys.Next,
			SessionHotkeyPrev:        hotkeys.Prev,
			SessionHotkeyClose:       hotkeys.Close,
		}, nil
	}
	connTimeout := s.Transfer.ConnectionTimeoutSec
	if connTimeout <= 0 {
		connTimeout = 15
	}
	maxConc := s.Transfer.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 4
	}
	hotkeys := s.SessionHotkeys
	defHotkeys := domain.DefaultSessionHotkeysSettings()
	if strings.TrimSpace(hotkeys.Create) == "" {
		hotkeys.Create = defHotkeys.Create
	}
	if strings.TrimSpace(hotkeys.Next) == "" {
		hotkeys.Next = defHotkeys.Next
	}
	if strings.TrimSpace(hotkeys.Prev) == "" {
		hotkeys.Prev = defHotkeys.Prev
	}
	if strings.TrimSpace(hotkeys.Close) == "" {
		hotkeys.Close = defHotkeys.Close
	}
	pingMode := s.Ping.Mode
	if pingMode == "" {
		pingMode = domain.PingModeInterval
	}
	return AppSettingsDTO{
		LockoutEnabled:           s.Lockout.Enabled,
		LockoutIdleMinutes:       int(s.Lockout.IdleTimeout.Minutes()),
		LockOnMinimize:           s.Lockout.LockOnMinimize,
		TerminalFontFamily:       s.Terminal.FontFamily,
		TerminalFontSize:         s.Terminal.FontSize,
		TerminalFontColor:        s.Terminal.FontColor,
		Theme:                    s.Theme,
		PingEnabled:              s.Ping.Enabled,
		PingMode:                 pingMode,
		PingIntervalSeconds:      s.Ping.EffectiveIntervalSeconds(),
		ExternalEditorPath:       s.ExternalEditorPath,
		TransferSpeedLimitKbps:   s.Transfer.SpeedLimitKbps,
		ConnectionTimeoutSeconds: connTimeout,
		MaxConcurrentTransfers:   maxConc,
		SessionHotkeyCreate:      hotkeys.Create,
		SessionHotkeyNext:        hotkeys.Next,
		SessionHotkeyPrev:        hotkeys.Prev,
		SessionHotkeyClose:       hotkeys.Close,
	}, nil
}

// SaveSettings persists application settings to the vault and applies them.
func (a *AppAPI) SaveSettings(dto AppSettingsDTO) error {
	data, err := a.vaultRepo.GetData()
	if err != nil {
		return err
	}
	if data.Settings == nil {
		data.Settings = &domain.AppSettings{}
	}

	data.Settings.Lockout = domain.LockoutSettings{
		Enabled:        dto.LockoutEnabled,
		IdleTimeout:    time.Duration(dto.LockoutIdleMinutes) * time.Minute,
		LockOnMinimize: dto.LockOnMinimize,
	}
	data.Settings.Terminal = domain.TerminalSettings{
		FontFamily: dto.TerminalFontFamily,
		FontSize:   dto.TerminalFontSize,
		FontColor:  dto.TerminalFontColor,
	}
	data.Settings.Theme = dto.Theme
	pingMode := dto.PingMode
	if pingMode != domain.PingModeInterval && pingMode != domain.PingModeOnChange {
		pingMode = domain.PingModeInterval
	}
	intervalSec := dto.PingIntervalSeconds
	if intervalSec < 1 {
		intervalSec = 5
	}
	data.Settings.Ping = domain.PingSettings{
		Enabled:         dto.PingEnabled,
		Mode:            pingMode,
		IntervalSeconds:  intervalSec,
	}
	data.Settings.ExternalEditorPath = dto.ExternalEditorPath
	data.Settings.Transfer = domain.TransferSettings{
		SpeedLimitKbps:       dto.TransferSpeedLimitKbps,
		ConnectionTimeoutSec: dto.ConnectionTimeoutSeconds,
		MaxConcurrent:        dto.MaxConcurrentTransfers,
	}
	defHotkeys := domain.DefaultSessionHotkeysSettings()
	createHotkey := strings.TrimSpace(dto.SessionHotkeyCreate)
	nextHotkey := strings.TrimSpace(dto.SessionHotkeyNext)
	prevHotkey := strings.TrimSpace(dto.SessionHotkeyPrev)
	closeHotkey := strings.TrimSpace(dto.SessionHotkeyClose)
	if createHotkey == "" {
		createHotkey = defHotkeys.Create
	}
	if nextHotkey == "" {
		nextHotkey = defHotkeys.Next
	}
	if prevHotkey == "" {
		prevHotkey = defHotkeys.Prev
	}
	if closeHotkey == "" {
		closeHotkey = defHotkeys.Close
	}
	data.Settings.SessionHotkeys = domain.SessionHotkeysSettings{
		Create: createHotkey,
		Next:   nextHotkey,
		Prev:   prevHotkey,
		Close:  closeHotkey,
	}

	if err := a.vaultRepo.SaveData(context.Background(), data); err != nil {
		return err
	}

	if a.lockout != nil {
		a.lockout.UpdateSettings(data.Settings.Lockout)
	}
	if a.pingMgr != nil {
		a.pingMgr.UpdateSettings(data.Settings.Ping)
		a.pingMgr.Stop()
		a.pingMgr.Start(func(results []usecase.PingResult) {
			if a.ctx != nil {
				wailsrt.EventsEmit(a.ctx, EventPingUpdated, results)
			}
		})
	}

	return nil
}

// --- Audit Log ---

// AuditEntryDTO is the UI-facing representation of an audit log entry.
type AuditEntryDTO struct {
	ID           int64  `json:"id"`
	Timestamp    string `json:"timestamp"`
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
	Username     string `json:"username"`
	Input        string `json:"input"`
	Redacted     bool   `json:"redacted"`
}

// SearchAuditLog performs full-text search on audit entries.
func (a *AppAPI) SearchAuditLog(query, sessionID, connectionID string, limit, offset int) ([]AuditEntryDTO, error) {
	if a.auditLog == nil {
		return nil, fmt.Errorf("audit log not available")
	}
	filter := domain.AuditSearchFilter{
		SessionID:    sessionID,
		ConnectionID: connectionID,
		Limit:        limit,
		Offset:       offset,
	}
	entries, err := a.auditLog.Search(context.Background(), query, filter)
	if err != nil {
		return nil, err
	}
	result := make([]AuditEntryDTO, len(entries))
	for i, e := range entries {
		result[i] = AuditEntryDTO{
			ID:           e.ID,
			Timestamp:    e.Timestamp.Format(time.RFC3339),
			SessionID:    e.SessionID,
			ConnectionID: e.ConnectionID,
			Username:     e.Username,
			Input:        e.Input,
			Redacted:     e.Redacted,
		}
	}
	return result, nil
}

// DeleteAuditEntry removes a single audit log entry by ID.
func (a *AppAPI) DeleteAuditEntry(id int64) error {
	if a.auditLog == nil {
		return fmt.Errorf("audit log not available")
	}
	return a.auditLog.DeleteByID(context.Background(), id)
}

// ClearAuditLog removes all audit log entries.
func (a *AppAPI) ClearAuditLog() error {
	if a.auditLog == nil {
		return fmt.Errorf("audit log not available")
	}
	return a.auditLog.ClearAll(context.Background())
}

// --- Host Key ---

// ResolveHostKey handles the user's decision on a pending host key verification.
// action is "add" or "replace"; after resolving, retries the session connection.
func (a *AppAPI) ResolveHostKey(sessionID, action, host, authorizedKey string) error {
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return fmt.Errorf("parse host key: %w", err)
	}

	switch action {
	case "add":
		if err := a.knownHosts.Add(context.Background(), host, key); err != nil {
			return fmt.Errorf("add host key: %w", err)
		}
	case "replace":
		if err := a.knownHosts.Replace(context.Background(), host, key); err != nil {
			return fmt.Errorf("replace host key: %w", err)
		}
	default:
		return fmt.Errorf("unknown host key action %q", action)
	}

	return a.sessions.RetrySession(sessionID)
}

// --- Internal helpers ---

func (a *AppAPI) onSessionStateChange(session domain.ConnectionSession) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventSessionStateChanged, SessionToDTO(session))
}

func (a *AppAPI) onHostKeyRequest(sessionID string, info domain.HostKeyInfo) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventHostKeyRequired, map[string]interface{}{
		"sessionId":   sessionID,
		"host":        info.Host,
		"keyType":     info.KeyType,
		"fingerprint": info.Fingerprint,
		"keyBase64":   info.KeyBase64,
		"mismatch":    info.Mismatch,
	})
}

func (a *AppAPI) onPassphraseRequest(identityID, comment string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context for passphrase request")
	}
	_, _ = wailsrt.MessageDialog(a.ctx, wailsrt.MessageDialogOptions{
		Type:    wailsrt.InfoDialog,
		Title:   "Passphrase Required",
		Message: fmt.Sprintf("Key '%s' requires a passphrase. This feature requires a custom dialog.", comment),
	})
	return "", domain.ErrPassphraseRequired
}

// onStreamReady is called when a Telnet/Serial connector has started the terminal bridge.
// It begins streaming output to the frontend.
func (a *AppAPI) onStreamReady(sessionID string, outputCh <-chan []byte) {
	go a.streamTerminalOutput(sessionID, outputCh)
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTerminalReady, map[string]interface{}{"sessionId": sessionID})
	}
}

// initSessionPTYAndSFTP polls until the session is ready, then sets up PTY and SFTP (SSH only).
// For Telnet/Serial the connector sets the bridge and calls OnStreamReady; for RDP/HTTP no terminal.
func (a *AppAPI) initSessionPTYAndSFTP(sessionID string) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var info domain.ConnectionSession
	for i := 0; i < 300; i++ {
		<-ticker.C
		var err error
		info, err = a.sessions.GetState(sessionID)
		if err != nil || info.State == domain.SessionError || info.State == domain.SessionClosed {
			return
		}
		if info.State == domain.SessionReady {
			break
		}
	}

	proto := info.Protocol
	if proto == "" {
		proto = domain.ProtocolSSH
	}
	if proto != domain.ProtocolSSH {
		return
	}

	sshClient, err := a.sessions.GetSSHClient(sessionID)
	if err != nil {
		return
	}

	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return
	}

	ptyBridge := infrassh.NewPTYBridge()
	outputCh, err := ptyBridge.Start(ctx, sshClient, domain.PTYOptions{
		Cols: 80, Rows: 24, Term: "xterm-256color",
	})
	if err != nil {
		return
	}
	a.sessions.SetPTYBridge(sessionID, ptyBridge)
	go a.streamTerminalOutput(sessionID, outputCh)

	sftpClient, err := infrasftp.NewSFTPClient(sshClient.Client())
	if err != nil {
		return
	}
	rateLimitKbps := 0
	if data, err := a.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.SpeedLimitKbps > 0 {
		rateLimitKbps = data.Settings.Transfer.SpeedLimitKbps
	}
	remoteFS := infrasftp.NewRemoteFSWithRateLimit(sftpClient, rateLimitKbps)
	a.sessions.SetRemoteFS(sessionID, remoteFS)

	initialPath := "/"
	if wd, err := remoteFS.GetWorkingDirectory(ctx); err == nil && wd != "" {
		initialPath = wd
	}

	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSFTPReady, map[string]interface{}{
			"sessionId":   sessionID,
			"initialPath": initialPath,
		})
	}
}

func (a *AppAPI) streamTerminalOutput(sessionID string, outputCh <-chan []byte) {
	sanitizer := a.getSanitizer(sessionID)
	batchTicker := time.NewTicker(50 * time.Millisecond)
	defer batchTicker.Stop()

	var batch []byte

	flush := func() {
		if len(batch) == 0 {
			return
		}
		sanitizer.FeedOutput(string(batch))
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTerminalOutput, TerminalOutputPayload{
				SessionID: sessionID,
				Output:    base64.StdEncoding.EncodeToString(batch),
			})
		}
		batch = batch[:0]
	}

	hasNewline := func(b []byte) bool {
		for _, c := range b {
			if c == '\n' || c == '\r' {
				return true
			}
		}
		return false
	}

	for {
		select {
		case data, ok := <-outputCh:
			if !ok {
				flush()
				a.sanitizersMu.Lock()
				delete(a.sanitizers, sessionID)
				a.sanitizersMu.Unlock()
				a.sessions.NotifySessionDisconnected(sessionID)
				return
			}
			batch = append(batch, data...)
			if hasNewline(batch) {
				flush()
			}
		case <-batchTicker.C:
			flush()
		}
	}
}
