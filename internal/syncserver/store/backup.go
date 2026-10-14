package store

import (
	"context"
	"fmt"
)

// BackupTo writes a consistent copy of the database to path with VACUUM INTO, which reads one
// snapshot while the server keeps serving and refuses to overwrite an existing file.
func (s *DB) BackupTo(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("store: backup: %w", err)
	}
	return nil
}
