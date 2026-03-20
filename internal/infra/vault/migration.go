package vault

import (
	"ssh-client/internal/domain"
)

// MigrateVaultData upgrades vault data to the current schema version in-place.
// Each step is additive and preserves all existing data.
func MigrateVaultData(data *domain.VaultData) {
	if data.Version < 2 {
		migrateV1ToV2(data)
	}
}

// migrateV1ToV2 upgrades a v1 vault:
//   - initialises new maps (Passwords, VPNProfiles)
//   - initialises Settings with defaults
//   - converts legacy Connection.User + IdentityIDs into a single ConnectionUser entry
//   - bumps version to 2
func migrateV1ToV2(data *domain.VaultData) {
	if data.Passwords == nil {
		data.Passwords = map[string]domain.PasswordBlob{}
	}
	if data.VPNProfiles == nil {
		data.VPNProfiles = map[string]domain.VPNProfile{}
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

	data.Version = domain.CurrentVaultVersion
}
