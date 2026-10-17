package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"xquakshell/internal/domain"
)

// Backups copies the vault aside before an operation that rewrites much of it at once, such as
// replacing this device's data with a sync account's. Unlike BackupVaultFile, which is keyed by the
// schema version a migration leaves behind, each copy here is named by its reason and instant, so a
// sync backup can never stand in for - or block - the pre-migration copy a later upgrade must make.
type Backups struct {
	Dir string
	Now func() time.Time
}

var _ domain.VaultBackup = Backups{}

// errBackupReason is a reason that would not make a safe, readable file name part.
var errBackupReason = errors.New("vault backup: reason must be lowercase letters, digits and dashes")

func validReason(r string) bool {
	if r == "" || len(r) > 32 {
		return false
	}
	for _, c := range r {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// BackupNow writes a copy of the vault file and returns its file name. The copy is the same
// age-encrypted bytes under the same master password, so it is as safe at rest as the vault. It is
// created exclusively: an existing backup is never replaced, and nothing deletes these copies.
func (b Backups) BackupNow(_ context.Context, reason string) (string, error) {
	if !validReason(reason) {
		return "", errBackupReason
	}
	ciphertext, err := os.ReadFile(FilePath(b.Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", domain.ErrVaultNotFound
	}
	if err != nil {
		return "", fmt.Errorf("vault backup read: %w", err)
	}
	name := fmt.Sprintf("%s.%s-%d.bak", vaultFileName, reason, b.Now().UnixMilli())
	// The name is built from a constant, a validated reason and an integer; Dir is the vault
	// directory from the composition root, never user input.
	// #nosec G304 G703 -- see above
	f, err := os.OpenFile(filepath.Join(b.Dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("vault backup create: %w", err)
	}
	if _, err := f.Write(ciphertext); err != nil {
		return "", errors.Join(fmt.Errorf("vault backup write: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("vault backup close: %w", err)
	}
	return name, nil
}
