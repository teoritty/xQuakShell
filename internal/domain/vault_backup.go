package domain

import "context"

// VaultBackup copies the vault file aside before an operation that rewrites much of it at once.
// reason names the copy; the returned name is the copy's file name, for the user to find it.
type VaultBackup interface {
	BackupNow(ctx context.Context, reason string) (string, error)
}
