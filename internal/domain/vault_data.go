package domain

// IdentityBlob holds the stored private key for one identity.
//
// From schema 4 the bytes are always an OpenSSH private key encrypted with bcrypt_pbkdf, and
// DataKey carries the random passphrase that unwraps it for a KeyPolicyVault identity. For a
// KeyPolicyPassphrase identity DataKey is empty and the passphrase comes from the user, so the
// vault holds nothing that opens the key on its own.
//
// PEMData keeps its JSON name because it is the persisted v3 field: a migration that renamed it
// would have to rewrite every key before the user could open the vault at all, including the
// ones whose passphrase was skipped.
type IdentityBlob struct {
	PEMData []byte `json:"pemData"`
	// DataKey unwraps PEMData for a KeyPolicyVault identity; empty under any other policy.
	DataKey []byte `json:"dataKey,omitempty"`
	// Legacy marks bytes still in their pre-v4 shape because migration skipped this key. Such a
	// blob is used as-is for authentication and refuses every operation that needs rewriting.
	Legacy bool `json:"legacy,omitempty"`
}

// PasswordBlob holds encrypted password bytes stored in the vault.
type PasswordBlob struct {
	Value []byte `json:"value"`
	Label string `json:"label"`
}

// CurrentVaultVersion is the latest vault data schema version.
const CurrentVaultVersion = 4

// MinMigratableVaultVersion is the oldest schema this build can upgrade in place. A vault below
// it is refused rather than guessed at, because the fields the migration reads are the ones that
// version is not guaranteed to have.
//
// It is a separate constant from CurrentVaultVersion on purpose: collapsing the two would make
// every future bump silently drop support for the version before it, and the whole point of a
// migration is that the previous version keeps opening.
const MinMigratableVaultVersion = 3

// VaultData is the top-level structure stored inside the encrypted vault file.
type VaultData struct {
	Version     int                     `json:"version"`
	Folders     []ConnectionFolder      `json:"folders"`
	Connections []Connection            `json:"connections"`
	Identities  map[string]SSHIdentity  `json:"identities"`
	KeyBlobs    map[string]IdentityBlob `json:"keyBlobs"`
	KnownHosts  []string                `json:"knownHosts"`

	Passwords     map[string]PasswordBlob `json:"passwords,omitempty"`
	PluginSecrets map[string][]byte       `json:"pluginSecrets,omitempty"`

	// PeerTrust is what plugins trust about the identity of a remote peer.
	//
	// Deliberately apart from KnownHosts. That field holds OpenSSH-format lines and is read by
	// the SSH path; one shared list would mean a plugin can plant a key for an SSH host.
	//
	// The field is additive and omitempty, so the schema version does not move: an older vault
	// opens as an empty list, and a newer one still reads on the previous build. Passwords and
	// PluginSecrets were added the same way.
	PeerTrust []PeerTrustEntry `json:"peerTrust,omitempty"`

	Settings *AppSettings `json:"settings,omitempty"`
}

// NewVaultData returns an empty VaultData at the current schema version.
func NewVaultData() *VaultData {
	return &VaultData{
		Version:       CurrentVaultVersion,
		Folders:       []ConnectionFolder{},
		Connections:   []Connection{},
		Identities:    map[string]SSHIdentity{},
		KeyBlobs:      map[string]IdentityBlob{},
		KnownHosts:    []string{},
		Passwords:     map[string]PasswordBlob{},
		PluginSecrets: map[string][]byte{},
		PeerTrust:     []PeerTrustEntry{},
		Settings: &AppSettings{
			Lockout:        DefaultLockoutSettings(),
			Terminal:       DefaultTerminalSettings(),
			Theme:          "dark",
			UIScalePercent: 100,
			Ping:           DefaultPingSettings(),
			Transfer:       DefaultTransferSettings(),
			SessionHotkeys: DefaultSessionHotkeysSettings(),
			LocalTerminal:  DefaultLocalTerminalSettings(),
			AuditLog:       DefaultAuditLogSettings(),
			Plugins:        DefaultPluginSettings(),
		},
	}
}
