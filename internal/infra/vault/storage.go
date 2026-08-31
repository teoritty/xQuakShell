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

// FilePath is where the vault lives inside a vault directory.
//
// Every read, write and backup below goes through it rather than joining the name again, so there
// is one answer to "which file is the vault" and a test can ask for it instead of hardcoding a
// second copy that would keep passing after the real one moved.
func FilePath(dir string) string {
	return filepath.Join(dir, vaultFileName)
}

// BackupPath is where a pre-migration copy of the vault lands, named after the version it holds.
//
// It is exported so the UI can tell the user exactly which file to keep, on the one screen where
// that matters: a migration is the only moment a user might need to go back to the old data, and
// "a backup was made somewhere" is not something they can act on.
func BackupPath(dir string, fromVersion int) string {
	return filepath.Join(dir, fmt.Sprintf("%s.v%d.bak", vaultFileName, fromVersion))
}

// Exists reports whether a vault file is present in dir.
func Exists(dir string) bool {
	_, err := os.Stat(FilePath(dir))
	return err == nil
}

// readVaultBytes reads the vault file without interpreting it.
//
// It returns domain.ErrVaultNotFound when no vault file exists: bringing a vault
// into existence is the explicit job of VaultRepo.Create, never a side effect of
// a read. Synthesizing an empty vault here would make a typo on the unlock
// screen indistinguishable from deliberately choosing a new master password.
func readVaultBytes(dir string) ([]byte, error) {
	path := FilePath(dir)

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrVaultNotFound
		}
		return nil, fmt.Errorf("vault read file %s: %w", path, err)
	}
	return raw, nil
}

// BackupVaultFile copies the vault aside before a schema migration rewrites it, naming the copy
// after the version being left behind: vault.age.v3.bak.
//
// It refuses to overwrite an existing backup. The file it would clobber is the last copy of the
// data as it stood before the first migration to run, and a second attempt - a crash mid-upgrade,
// a downgrade and re-upgrade - would replace that original with already-migrated bytes, which is
// the one thing a backup must never do. Nothing deletes these: they hold someone's keys, and an
// application that silently disposes of the only pre-migration copy has no way to be right.
//
// The copy carries no plaintext; it is the same age-encrypted file under the same master
// password, so it is exactly as safe at rest as the vault itself.
func BackupVaultFile(dir string, fromVersion int) error {
	source := FilePath(dir)
	target := BackupPath(dir, fromVersion)

	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("vault backup stat %s: %w", target, err)
	}

	ciphertext, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.ErrVaultNotFound
		}
		return fmt.Errorf("vault backup read %s: %w", source, err)
	}

	// gosec traces dir into a write and cannot see that this is the same dir WriteVaultFile
	// already writes vault.age into, so the backup reaches nowhere the vault itself does not.
	// The filename is built from a constant and an int, and the read above has already proven a
	// vault exists here — the write only ever lands beside a file this package owns.
	// #nosec G703 -- dir is the vault directory from the composition root, never user input
	if err := os.WriteFile(target, ciphertext, 0o600); err != nil {
		return fmt.Errorf("vault backup write %s: %w", target, err)
	}
	return nil
}

// writeVaultBytes atomically replaces the vault file.
// It writes to a temporary file first, syncs, then renames to the final name, so a crash mid-write
// leaves either the previous vault or the new one and never a truncated file that opens with
// neither credential.
func writeVaultBytes(dir string, ciphertext []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("vault mkdir %s: %w", dir, err)
	}

	tmpPath := filepath.Join(dir, vaultTmpName)
	finalPath := FilePath(dir)

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
