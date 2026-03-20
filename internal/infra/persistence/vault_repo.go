package persistence

import (
	"context"
	"fmt"
	"sync"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/vault"
)

// VaultRepo implements domain.VaultRepository backed by an age-encrypted file.
type VaultRepo struct {
	mu         sync.RWMutex
	dir        string
	passphrase string
	data       *domain.VaultData
	unlocked   bool
}

// NewVaultRepo creates a new VaultRepo that stores vault.age in the given directory.
func NewVaultRepo(dir string) *VaultRepo {
	return &VaultRepo{dir: dir}
}

// Unlock decrypts the vault with the given master password.
// If the vault file does not exist, a new empty vault is created and saved.
func (r *VaultRepo) Unlock(_ context.Context, masterPassword string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := vault.ReadVaultFile(r.dir, masterPassword)
	if err != nil {
		return err
	}

	if data.Identities == nil {
		data.Identities = map[string]domain.SSHIdentity{}
	}
	if data.KeyBlobs == nil {
		data.KeyBlobs = map[string]domain.IdentityBlob{}
	}
	if data.Passwords == nil {
		data.Passwords = map[string]domain.PasswordBlob{}
	}
	if data.VPNProfiles == nil {
		data.VPNProfiles = map[string]domain.VPNProfile{}
	}
	if data.Settings == nil {
		data.Settings = &domain.AppSettings{
			Lockout:  domain.DefaultLockoutSettings(),
			Terminal: domain.DefaultTerminalSettings(),
			Theme:    "dark",
		}
	}
	if data.Settings.Terminal.FontFamily == "" {
		data.Settings.Terminal = domain.DefaultTerminalSettings()
	}
	if data.Settings.Theme == "" {
		data.Settings.Theme = "dark"
	}

	if err := vault.WriteVaultFile(r.dir, masterPassword, data); err != nil {
		return fmt.Errorf("vault initial write: %w", err)
	}

	r.passphrase = masterPassword
	r.data = data
	r.unlocked = true
	return nil
}

// Lock clears the decrypted data and passphrase from memory.
func (r *VaultRepo) Lock() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = nil
	r.passphrase = ""
	r.unlocked = false
}

// IsUnlocked returns true when the vault is decrypted in memory.
func (r *VaultRepo) IsUnlocked() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.unlocked
}

// GetData returns the current in-memory vault data.
func (r *VaultRepo) GetData() (*domain.VaultData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.unlocked {
		return nil, domain.ErrVaultLocked
	}
	return r.data, nil
}

// SaveData persists the vault data to the encrypted file and updates the in-memory copy.
func (r *VaultRepo) SaveData(_ context.Context, data *domain.VaultData) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.unlocked {
		return domain.ErrVaultLocked
	}

	if err := vault.WriteVaultFile(r.dir, r.passphrase, data); err != nil {
		return err
	}

	r.data = data
	return nil
}
