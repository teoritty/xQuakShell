package domain

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

// VaultRepository provides access to the encrypted vault storage.
// GetData returns a deep snapshot; mutations must go through UpdateData.
//
// Create is the only way a vault comes into existence — Unlock never creates
// one — and it leaves the vault unlocked on success. Exists is an advisory
// probe for the UI to choose between the create and unlock screens; the
// authoritative check happens inside Create.
type VaultRepository interface {
	Exists() bool
	Create(ctx context.Context, masterPassword string) error
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

// PublicKey is a type alias for ssh.PublicKey.
type PublicKey = ssh.PublicKey

// AuthMethod is a type alias for ssh.AuthMethod, allowing usecase to hold
// this type without importing golang.org/x/crypto/ssh directly.
type AuthMethod = ssh.AuthMethod

// TunnelChannelDialer opens channels multiplexed over an already-established
// SSH connection (direct-tcpip / tcpip-forward), used for port forwarding
// without a second TCP connection or re-authentication.
type TunnelChannelDialer interface {
	OpenDirectTCP(ctx context.Context, addr string) (net.Conn, error)
	ListenTCP(ctx context.Context, remoteAddr string) (net.Listener, error)
}

// SSHClient wraps an active SSH connection with session creation capability.
type SSHClient interface {
	TunnelChannelDialer
	NewSession() (*ssh.Session, error)
	Client() *ssh.Client
	Close() error
	KeepAlive() error
}

// SSHClientConfig holds all parameters needed to establish an SSH connection.
type SSHClientConfig struct {
	Host             string
	Port             int
	User             string
	Signers          []ssh.Signer
	Password         string
	ExtraAuthMethods []AuthMethod // plugin-provided auth (keyboard-interactive, remote signer)
	HostKeyCallback  ssh.HostKeyCallback
	TimeoutSeconds   int
	Transport        net.Conn
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
//
// GetKeyBlob returns the stored wrapped bytes and is deliberately not enough to use a key: the
// caller still needs the data key or the user's passphrase. Callers that want a usable key go
// through the use case, not here.
type IdentityRepository interface {
	GetAll(ctx context.Context) ([]SSHIdentity, error)
	Get(ctx context.Context, id string) (*SSHIdentity, error)
	GetKeyBlob(ctx context.Context, id string) ([]byte, error)
	GetBlob(ctx context.Context, id string) (*IdentityBlob, error)
	Import(ctx context.Context, pemData []byte, comment string) (*SSHIdentity, error)
	Save(ctx context.Context, identity SSHIdentity, blob IdentityBlob) error
	Update(ctx context.Context, id string, mutate func(*SSHIdentity) error) error
	Delete(ctx context.Context, id string) error
}

// KeyMaterial is a private key parsed out of its stored form, together with the metadata derived
// from it. PEM holds the key in the vault's storage shape, never in cleartext.
type KeyMaterial struct {
	PEM         []byte
	PublicKey   string
	Fingerprint string
	KeyType     string
	Bits        int
}

// KeyCodec converts private keys between the shape the vault stores and the shapes the outside
// world uses. Implemented in infra over golang.org/x/crypto/ssh so the domain does not have to
// know the wire format.
//
// Wrap and Unwrap both speak the OpenSSH bcrypt_pbkdf format in either direction, so a key that
// leaves through Export is a file ssh-keygen would have written.
type KeyCodec interface {
	// Generate creates a new private key wrapped under the given passphrase.
	Generate(spec GeneratedKeySpec, passphrase []byte) (*KeyMaterial, error)
	// Normalize re-wraps an imported key under the given passphrase, reading it with oldPassphrase
	// (empty when the import is unprotected). It returns ErrPassphraseRequired when the input is
	// encrypted and oldPassphrase is empty.
	Normalize(pemData, oldPassphrase, passphrase []byte, comment string) (*KeyMaterial, error)
	// Unwrap parses stored bytes into a signer.
	Unwrap(pemData, passphrase []byte) (Signer, error)
	// Describe reads public metadata out of stored bytes without needing the passphrase.
	Describe(pemData []byte) (keyType string, encrypted bool)
	// Export re-wraps a key for writing outside the vault. An empty passphrase produces an
	// unprotected key, which is the user's explicit choice at the export screen.
	Export(pemData, passphrase, exportPassphrase []byte, comment string) ([]byte, error)
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

type CommandLineTracker interface {
	Feed(data string) (submitted string, ok bool)
}

type CommandLineTrackerFactory func() CommandLineTracker
