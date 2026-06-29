package domain

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

// VaultRepository provides access to the encrypted vault storage.
// GetData returns a deep snapshot; mutations must go through UpdateData.
type VaultRepository interface {
	Unlock(ctx context.Context, masterPassword string) error
	Lock()
	IsUnlocked() bool
	GetData() (*VaultData, error)
	UpdateData(ctx context.Context, mutate func(*VaultData) error) error
}

// ConnectionRepository manages connections and folders persisted in the vault.
type ConnectionRepository interface {
	GetAllFolders(ctx context.Context) ([]ConnectionFolder, error)
	SaveFolder(ctx context.Context, f *ConnectionFolder) error
	DeleteFolder(ctx context.Context, id string) error
	GetAllConnections(ctx context.Context) ([]Connection, error)
	GetByFolder(ctx context.Context, folderID string) ([]Connection, error)
	GetByID(ctx context.Context, id string) (*Connection, error)
	Save(ctx context.Context, c *Connection) error
	Delete(ctx context.Context, id string) error
	MoveToFolder(ctx context.Context, connectionIDs []string, folderID string) error
	MoveFolder(ctx context.Context, folderID, targetParentID string) error
	ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error
	ReorderFolders(ctx context.Context, folderIDs []string, parentID string) error
}

// KnownHostEntry represents a single entry in the known_hosts list for display in UI.
type KnownHostEntry struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	Line        string `json:"line"`
}

// KnownHostsRepository manages SSH known host entries inside the vault.
type KnownHostsRepository interface {
	Check(host string, remoteKey ssh.PublicKey) error
	Add(ctx context.Context, host string, key ssh.PublicKey) error
	List() ([]KnownHostEntry, error)
	Remove(ctx context.Context, host string) error
	Replace(ctx context.Context, host string, newKey ssh.PublicKey) error
}

// Signer is a type alias for ssh.Signer, allowing usecase to reference this type
// through domain without importing golang.org/x/crypto/ssh directly.
type Signer = ssh.Signer

// SSHClient wraps an active SSH connection with session creation capability.
type SSHClient interface {
	NewSession() (*ssh.Session, error)
	Client() *ssh.Client
	Close() error
	KeepAlive() error
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
	Transport       net.Conn
}

// SSHClientFactory creates SSH connections from a configuration.
type SSHClientFactory interface {
	Create(ctx context.Context, cfg SSHClientConfig) (SSHClient, error)
}

// PTYOptions configures the pseudo-terminal request for a session.
type PTYOptions struct {
	Cols    uint32
	Rows    uint32
	Term    string
	Command string
}

// TerminalPTYBridge manages a PTY session over SSH, streaming I/O to/from the frontend.
type TerminalPTYBridge interface {
	Start(ctx context.Context, sshClient SSHClient, opts PTYOptions) (<-chan []byte, error)
	Write(data []byte) error
	Resize(cols, rows uint32) error
	Close() error
}

// PTYBridgeFactory creates new TerminalPTYBridge instances.
type PTYBridgeFactory interface {
	NewBridge() TerminalPTYBridge
}

// SFTPClientFactory creates RemoteFS adapters from an active SSH connection.
type SFTPClientFactory interface {
	New(client SSHClient, rateLimitKbps int) (RemoteFS, error)
}

// IdentityRepository manages SSH identity (private key) entries in the vault.
type IdentityRepository interface {
	GetAll(ctx context.Context) ([]SSHIdentity, error)
	GetKeyBlob(ctx context.Context, id string) ([]byte, error)
	Import(ctx context.Context, pemData []byte, comment string) (*SSHIdentity, error)
	Delete(ctx context.Context, id string) error
}

// PasswordRepository manages encrypted password entries in the vault.
type PasswordRepository interface {
	Import(ctx context.Context, password []byte, label string) (string, error)
	Get(ctx context.Context, id string) ([]byte, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]PasswordBlob, error)
}

// LockoutEventHandler is called when the lockout triggers.
type LockoutEventHandler func()

// LockoutManager monitors user activity and triggers vault lockout on timeout.
type LockoutManager interface {
	Start(handler LockoutEventHandler)
	Stop()
	ReportActivity()
	ReportMinimized()
	ReportRestored()
	UpdateSettings(settings LockoutSettings)
	GetSettings() LockoutSettings
}

// HostKeyInfo carries host key details for display in the UI during verification prompts.
type HostKeyInfo struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	KeyBase64   string `json:"keyBase64"`
	Mismatch    bool   `json:"mismatch"`
}

// CommandLineTracker reconstructs shell command lines from PTY keystrokes.
type CommandLineTracker interface {
	Feed(data string) (submitted string, ok bool)
}

// CommandLineTrackerFactory creates a new command line tracker instance.
type CommandLineTrackerFactory func() CommandLineTracker
