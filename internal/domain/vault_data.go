package domain

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
const CurrentVaultVersion = 3

// VaultData is the top-level structure stored inside the encrypted vault file.
type VaultData struct {
	Version     int                     `json:"version"`
	Folders     []ConnectionFolder      `json:"folders"`
	Connections []Connection            `json:"connections"`
	Identities  map[string]SSHIdentity  `json:"identities"`
	KeyBlobs    map[string]IdentityBlob `json:"keyBlobs"`
	KnownHosts  []string                `json:"knownHosts"`

	// v2 additions
	Passwords map[string]PasswordBlob `json:"passwords,omitempty"`
	Settings  *AppSettings            `json:"settings,omitempty"`
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
		Settings: &AppSettings{
			Lockout:        DefaultLockoutSettings(),
			Terminal:       DefaultTerminalSettings(),
			Theme:          "dark",
			UIScalePercent: 100,
			Ping:           DefaultPingSettings(),
			Transfer:       DefaultTransferSettings(),
			SessionHotkeys: DefaultSessionHotkeysSettings(),
			AuditLog:       DefaultAuditLogSettings(),
			Plugins:        DefaultPluginSettings(),
		},
	}
}
