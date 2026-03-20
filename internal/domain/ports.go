package domain

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

// --- Vault ---

// IdentityBlob holds the raw PEM bytes for a private key stored in the vault.
type IdentityBlob struct {
	PEMData []byte `json:"pemData"`
}

// PasswordBlob holds encrypted password bytes stored in the vault.
type PasswordBlob struct {
	Value []byte `json:"value"`
	Label string `json:"label"`
}

// CurrentVaultVersion is the latest vault data schema version.
const CurrentVaultVersion = 2

// VaultData is the top-level structure stored inside the encrypted vault file.
type VaultData struct {
	Version     int                      `json:"version"`
	Folders     []ConnectionFolder       `json:"folders"`
	Connections []Connection             `json:"connections"`
	Identities  map[string]SSHIdentity   `json:"identities"`
	KeyBlobs    map[string]IdentityBlob  `json:"keyBlobs"`
	KnownHosts  []string                 `json:"knownHosts"`

	// v2 additions
	Passwords   map[string]PasswordBlob  `json:"passwords,omitempty"`
	VPNProfiles map[string]VPNProfile    `json:"vpnProfiles,omitempty"`
	Settings    *AppSettings             `json:"settings,omitempty"`
}

// TerminalSettings configures the embedded terminal appearance.
type TerminalSettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	FontColor  string `json:"fontColor"`
}

// DefaultTerminalSettings returns sensible terminal defaults.
func DefaultTerminalSettings() TerminalSettings {
	return TerminalSettings{
		FontFamily: "Cascadia Code, Consolas, Courier New, monospace",
		FontSize:   14,
		FontColor:  "#cccccc",
	}
}

// PingMode controls when ping runs: "interval" = periodic, "on_change" = only when connection settings change.
const (
	PingModeInterval  = "interval"
	PingModeOnChange  = "on_change"
)

// PingSettings configures automatic host reachability checks.
type PingSettings struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`             // "interval" or "on_change"
	IntervalSeconds int    `json:"intervalSeconds"`   // used when mode=interval; default 5
	IntervalMinutes int    `json:"intervalMinutes"`  // deprecated, migrated to intervalSeconds
}

// EffectiveIntervalSeconds returns the interval in seconds, migrating from legacy intervalMinutes if needed.
func (p PingSettings) EffectiveIntervalSeconds() int {
	if p.IntervalSeconds > 0 {
		return p.IntervalSeconds
	}
	if p.IntervalMinutes > 0 {
		return p.IntervalMinutes * 60
	}
	return 5
}

// DefaultPingSettings returns reasonable defaults for ping monitoring.
func DefaultPingSettings() PingSettings {
	return PingSettings{
		Enabled:         true,
		Mode:            PingModeInterval,
		IntervalSeconds: 5,
	}
}

// TransferSettings configures file transfer behavior.
type TransferSettings struct {
	SpeedLimitKbps       int `json:"speedLimitKbps"`       // 0 = unlimited
	ConnectionTimeoutSec int `json:"connectionTimeoutSec"` // SSH timeout
	MaxConcurrent        int `json:"maxConcurrent"`       // max parallel transfers
}

// DefaultTransferSettings returns reasonable defaults.
func DefaultTransferSettings() TransferSettings {
	return TransferSettings{
		SpeedLimitKbps:       0,
		ConnectionTimeoutSec: 15,
		MaxConcurrent:        4,
	}
}

// SessionHotkeysSettings configures keyboard shortcuts for session management.
type SessionHotkeysSettings struct {
	Create string `json:"create"` // default: Ctrl+Shift+N
	Next   string `json:"next"`   // default: Ctrl+Tab
	Prev   string `json:"prev"`   // default: Ctrl+Shift+Tab
	Close  string `json:"close"`  // default: Ctrl+Shift+Q
}

// DefaultSessionHotkeysSettings returns default shortcuts for session management.
func DefaultSessionHotkeysSettings() SessionHotkeysSettings {
	return SessionHotkeysSettings{
		Create: "Ctrl+Shift+N",
		Next:   "Ctrl+Tab",
		Prev:   "Ctrl+Shift+Tab",
		Close:  "Ctrl+Shift+Q",
	}
}

// AppSettings stores user-configurable application settings inside the vault.
type AppSettings struct {
	Lockout            LockoutSettings   `json:"lockout"`
	Terminal           TerminalSettings  `json:"terminal"`
	Theme              string            `json:"theme"`
	Ping               PingSettings      `json:"ping"`
	Transfer           TransferSettings  `json:"transfer"`
	SessionHotkeys     SessionHotkeysSettings `json:"sessionHotkeys"`
	ExternalEditorPath string            `json:"externalEditorPath,omitempty"`
}

// NewVaultData returns an empty VaultData at the current schema version.
func NewVaultData() *VaultData {
	return &VaultData{
		Version:     CurrentVaultVersion,
		Folders:     []ConnectionFolder{},
		Connections: []Connection{},
		Identities:  map[string]SSHIdentity{},
		KeyBlobs:    map[string]IdentityBlob{},
		KnownHosts:  []string{},
		Passwords:   map[string]PasswordBlob{},
		VPNProfiles: map[string]VPNProfile{},
		Settings: &AppSettings{
			Lockout:        DefaultLockoutSettings(),
			Terminal:       DefaultTerminalSettings(),
			Theme:          "dark",
			Ping:           DefaultPingSettings(),
			Transfer:       DefaultTransferSettings(),
			SessionHotkeys: DefaultSessionHotkeysSettings(),
		},
	}
}

// VaultRepository provides access to the encrypted vault storage.
type VaultRepository interface {
	// Unlock decrypts the vault with the given master password.
	// If the vault file does not exist, it creates a new one.
	Unlock(ctx context.Context, masterPassword string) error

	// Lock re-locks the vault, clearing sensitive data from memory.
	Lock()

	// IsUnlocked returns true when the vault is currently decrypted in memory.
	IsUnlocked() bool

	// GetData returns the current in-memory vault data. Requires Unlock first.
	GetData() (*VaultData, error)

	// SaveData writes the given vault data to the encrypted vault file.
	SaveData(ctx context.Context, data *VaultData) error
}

// --- Connections ---

// ConnectionRepository manages connections and folders persisted in the vault.
type ConnectionRepository interface {
	// GetAllFolders returns every folder stored in the vault.
	GetAllFolders(ctx context.Context) ([]ConnectionFolder, error)

	// SaveFolder creates or updates a folder. ID must be set for updates.
	SaveFolder(ctx context.Context, f *ConnectionFolder) error

	// DeleteFolder removes a folder by ID.
	DeleteFolder(ctx context.Context, id string) error

	// GetAllConnections returns all connections regardless of folder.
	GetAllConnections(ctx context.Context) ([]Connection, error)

	// GetByFolder returns connections belonging to a specific folder.
	GetByFolder(ctx context.Context, folderID string) ([]Connection, error)

	// GetByID returns a single connection by its ID.
	GetByID(ctx context.Context, id string) (*Connection, error)

	// Save creates or updates a connection. ID must be set for updates.
	Save(ctx context.Context, c *Connection) error

	// Delete removes a connection by ID.
	Delete(ctx context.Context, id string) error

	// MoveToFolder moves one or more connections into a target folder.
	MoveToFolder(ctx context.Context, connectionIDs []string, folderID string) error

	// MoveFolder moves a folder to become a child of a different parent folder.
	// targetParentID="" moves the folder to root. Returns ErrCircularFolder on cycles.
	MoveFolder(ctx context.Context, folderID, targetParentID string) error

	// ReorderConnections updates the Order field of connections in folderID to match the given order.
	// connectionIDs lists connections in desired order (first = order 0, etc).
	ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error

	// ReorderFolders updates the Order field of folders under parentID to match the given order.
	// folderIDs lists folders in desired order.
	ReorderFolders(ctx context.Context, folderIDs []string, parentID string) error
}

// --- Known Hosts ---

// KnownHostEntry represents a single entry in the known_hosts list for display in UI.
type KnownHostEntry struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	Line        string `json:"line"`
}

// KnownHostsRepository manages SSH known host entries inside the vault.
type KnownHostsRepository interface {
	// Check verifies the remote host key against known_hosts.
	// Returns nil if the key matches, ErrUnknownHost if missing, ErrHostKeyMismatch if different.
	Check(host string, remoteKey ssh.PublicKey) error

	// Add stores a new host key entry in known_hosts and persists the vault.
	Add(ctx context.Context, host string, key ssh.PublicKey) error

	// List returns all known host entries for UI display.
	List() ([]KnownHostEntry, error)

	// Remove deletes a known host entry by host pattern.
	Remove(ctx context.Context, host string) error

	// Replace removes an existing entry and adds the new key (for mismatch resolution).
	Replace(ctx context.Context, host string, newKey ssh.PublicKey) error
}

// --- SSH ---

// SSHClient wraps an active SSH connection with session creation capability.
type SSHClient interface {
	// NewSession opens a new SSH channel session for shell/exec.
	NewSession() (*ssh.Session, error)

	// Client returns the underlying ssh.Client for SFTP initialization.
	Client() *ssh.Client

	// Close terminates the SSH connection.
	Close() error
}

// ProxyAuth holds SOCKS proxy authentication.
type ProxyAuth struct {
	Host     string
	Port     int
	Username string
	Password string
}

// SSHClientConfig holds all parameters needed to establish an SSH connection.
type SSHClientConfig struct {
	Host            string
	Port            int
	User            string
	Signers         []ssh.Signer
	Password        string
	HostKeyCallback ssh.HostKeyCallback
	TimeoutSeconds  int
	// Transport, when non-nil, is used instead of direct TCP dial (e.g. bastion channel).
	Transport net.Conn
	// Proxy, when non-nil, routes TCP through SOCKS before connecting (or before JumpChain).
	Proxy *ProxyAuth
}

// SSHClientFactory creates SSH connections from a configuration.
type SSHClientFactory interface {
	// Create establishes an SSH connection using the provided config.
	// When cfg.Transport is set, the connection is established over that net.Conn.
	Create(ctx context.Context, cfg SSHClientConfig) (SSHClient, error)
}

// --- Terminal ---

// PTYOptions configures the pseudo-terminal request for a session.
type PTYOptions struct {
	Cols    uint32
	Rows    uint32
	Term    string
	Command string
}

// TerminalPTYBridge manages a PTY session over SSH, streaming I/O to/from the frontend.
type TerminalPTYBridge interface {
	// Start opens a PTY on the remote and begins reading/writing.
	// outputCh receives raw bytes from stdout; the caller must drain it.
	Start(ctx context.Context, sshClient SSHClient, opts PTYOptions) (<-chan []byte, error)

	// Write sends input bytes to the remote PTY stdin.
	Write(data []byte) error

	// Resize changes the PTY window size.
	Resize(cols, rows uint32) error

	// Close terminates the PTY session and releases resources.
	Close() error
}

// --- Identity management ---

// IdentityRepository manages SSH identity (private key) entries in the vault.
type IdentityRepository interface {
	// GetAll returns metadata for every identity in the vault.
	GetAll(ctx context.Context) ([]SSHIdentity, error)

	// GetKeyBlob returns the raw PEM bytes for a given identity ID.
	GetKeyBlob(ctx context.Context, id string) ([]byte, error)

	// Import stores a new identity (PEM bytes + metadata) in the vault.
	Import(ctx context.Context, pemData []byte, comment string) (*SSHIdentity, error)

	// Delete removes an identity by ID.
	Delete(ctx context.Context, id string) error
}

// --- Passwords ---

// PasswordRepository manages encrypted password entries in the vault.
type PasswordRepository interface {
	// Import stores a new password and returns its generated ID.
	Import(ctx context.Context, password []byte, label string) (string, error)
	// Get retrieves a password by ID.
	Get(ctx context.Context, id string) ([]byte, error)
	// Delete removes a password by ID.
	Delete(ctx context.Context, id string) error
	// List returns all password metadata (ID + label, not the actual password).
	List(ctx context.Context) ([]PasswordBlob, error)
}

// --- VPN Profiles ---

// VPNProfileRepository manages VPN profile entries in the vault.
type VPNProfileRepository interface {
	// Save creates or updates a VPN profile in the vault.
	Save(ctx context.Context, profile *VPNProfile) error
	// Get retrieves a VPN profile by ID.
	Get(ctx context.Context, id string) (*VPNProfile, error)
	// Delete removes a VPN profile by ID.
	Delete(ctx context.Context, id string) error
	// GetAll returns every VPN profile stored in the vault.
	GetAll(ctx context.Context) ([]VPNProfile, error)
}

// --- Transport ---

// TransportDialer abstracts TCP-level connectivity, allowing bastion/VPN composition.
type TransportDialer interface {
	// DialContext establishes a network connection (TCP) to the given address.
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// --- Lockout ---

// LockoutEventHandler is called when the lockout triggers.
type LockoutEventHandler func()

// LockoutManager monitors user activity and triggers vault lockout on timeout.
type LockoutManager interface {
	// Start begins monitoring. handler is called when lockout triggers.
	Start(handler LockoutEventHandler)
	// Stop ceases monitoring.
	Stop()
	// ReportActivity resets the idle timer.
	ReportActivity()
	// ReportMinimized signals that the application was minimized.
	ReportMinimized()
	// ReportRestored signals that the application was restored from minimized.
	ReportRestored()
	// UpdateSettings applies new lockout configuration.
	UpdateSettings(settings LockoutSettings)
	// GetSettings returns current lockout configuration.
	GetSettings() LockoutSettings
}

// --- Host Key Info ---

// HostKeyInfo carries host key details for display in the UI during verification prompts.
type HostKeyInfo struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	KeyBase64   string `json:"keyBase64"`
	Mismatch    bool   `json:"mismatch"`
}
