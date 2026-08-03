package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"xquakshell/internal/domain"
)

const (
	vaultFileName = "vault.age"
	vaultTmpName  = "vault.age.tmp"
)

// Exists reports whether a vault file is present in dir.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, vaultFileName))
	return err == nil
}

// ReadVaultFile reads and decrypts the vault from disk.
// It returns domain.ErrVaultNotFound when no vault file exists: bringing a vault
// into existence is the explicit job of VaultRepo.Create, never a side effect of
// a read. Synthesizing an empty vault here would make a typo on the unlock
// screen indistinguishable from deliberately choosing a new master password.
func ReadVaultFile(dir, passphrase string) (*domain.VaultData, error) {
	path := filepath.Join(dir, vaultFileName)

	ciphertext, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrVaultNotFound
		}
		return nil, fmt.Errorf("vault read file %s: %w", path, err)
	}

	return Decrypt(ciphertext, passphrase)
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
