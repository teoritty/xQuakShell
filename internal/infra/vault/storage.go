package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"ssh-client/internal/domain"
)

const (
	vaultFileName = "vault.age"
	vaultTmpName  = "vault.age.tmp"
)

// ReadVaultFile reads and decrypts the vault from disk.
// If the file does not exist, returns a fresh VaultData at the current schema version.
// After decryption, the data is migrated to the latest schema if needed.
func ReadVaultFile(dir, passphrase string) (*domain.VaultData, error) {
	path := filepath.Join(dir, vaultFileName)

	ciphertext, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.NewVaultData(), nil
		}
		return nil, fmt.Errorf("vault read file %s: %w", path, err)
	}

	data, err := Decrypt(ciphertext, passphrase)
	if err != nil {
		return nil, err
	}

	MigrateVaultData(data)

	return data, nil
}

// WriteVaultFile encrypts and atomically writes the vault to disk.
// It writes to a temporary file first, syncs, then renames to the final name.
func WriteVaultFile(dir, passphrase string, data *domain.VaultData) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("vault mkdir %s: %w", dir, err)
	}

	ciphertext, err := Encrypt(data, passphrase)
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, vaultTmpName)
	finalPath := filepath.Join(dir, vaultFileName)

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("vault create tmp: %w", err)
	}

	if _, err := f.Write(ciphertext); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("vault write tmp: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("vault sync tmp: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("vault close tmp: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("vault rename: %w", err)
	}

	return nil
}
