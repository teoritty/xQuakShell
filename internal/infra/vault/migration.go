package vault

import (
	"encoding/json"

	"ssh-client/internal/domain"
)

// MigrateVaultData upgrades vault data to the current schema version in-place.
// Each step is additive and preserves all existing data.
func MigrateVaultData(data *domain.VaultData) {
	if data.Version < 2 {
		migrateV1ToV2(data)
	}
	if data.Version < 3 {
		migrateV2ToV3(data)
	}
}

// migrateV1ToV2 upgrades a v1 vault:
//   - initialises new maps (Passwords)
//   - initialises Settings with defaults
//   - converts legacy Connection.User + IdentityIDs into a single ConnectionUser entry
//   - bumps version to 2
func migrateV1ToV2(data *domain.VaultData) {
	if data.Passwords == nil {
		data.Passwords = map[string]domain.PasswordBlob{}
	}
	if data.Settings == nil {
		lockout := domain.DefaultLockoutSettings()
		data.Settings = &domain.AppSettings{Lockout: lockout}
	}

	for i := range data.Connections {
		c := &data.Connections[i]
		if len(c.Users) > 0 {
			continue
		}
		if c.User == "" {
			continue
		}

		uid := "migrated-" + c.ID
		u := domain.ConnectionUser{
			ID:       uid,
			Username: c.User,
			Auth:     domain.AuthMethodKey,
		}
		if len(c.IdentityIDs) > 0 {
			u.KeyAuth = &domain.KeyAuthConfig{IdentityIDs: c.IdentityIDs}
		}
		c.Users = []domain.ConnectionUser{u}
		c.DefaultUserID = uid
	}

	data.Version = 2
}

// migrateV2ToV3 removes legacy VPN data from the vault schema.
func migrateV2ToV3(data *domain.VaultData) {
	data.Version = domain.CurrentVaultVersion
}

// vaultDataLegacy supports unmarshaling v1/v2 vault JSON that may contain VPN fields.
type vaultDataLegacy struct {
	Version     int                           `json:"version"`
	Folders     []domain.ConnectionFolder     `json:"folders"`
	Connections []connectionLegacy            `json:"connections"`
	Identities  map[string]domain.SSHIdentity `json:"identities"`
	KeyBlobs    map[string]domain.IdentityBlob `json:"keyBlobs"`
	KnownHosts  []string                      `json:"knownHosts"`
	Passwords   map[string]domain.PasswordBlob `json:"passwords,omitempty"`
	VPNProfiles map[string]json.RawMessage    `json:"vpnProfiles,omitempty"`
	Settings    *domain.AppSettings           `json:"settings,omitempty"`
}

type connectionLegacy struct {
	domain.Connection
	VpnProfileID string `json:"vpnProfileId,omitempty"`
}

// UnmarshalLegacyVault parses v1/v2 vault JSON that may contain removed VPN fields.
func UnmarshalLegacyVault(plaintext []byte) (*domain.VaultData, error) {
	return unmarshalVaultLegacy(plaintext)
}

func unmarshalVaultLegacy(plaintext []byte) (*domain.VaultData, error) {
	var legacy vaultDataLegacy
	if err := json.Unmarshal(plaintext, &legacy); err != nil {
		return nil, err
	}

	connections := make([]domain.Connection, len(legacy.Connections))
	for i, c := range legacy.Connections {
		connections[i] = c.Connection
	}

	return &domain.VaultData{
		Version:     legacy.Version,
		Folders:     legacy.Folders,
		Connections: connections,
		Identities:  legacy.Identities,
		KeyBlobs:    legacy.KeyBlobs,
		KnownHosts:  legacy.KnownHosts,
		Passwords:   legacy.Passwords,
		Settings:    legacy.Settings,
	}, nil
}
