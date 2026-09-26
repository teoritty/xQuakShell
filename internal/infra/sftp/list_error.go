package sftp

import (
	"errors"
	"fmt"
	"os"

	"xquakshell/internal/domain"
)

// listError names a failed directory listing in domain terms.
//
// A missing directory gets its own error, without the path or the SFTP status: the file pane
// answers it by stepping up to a parent that still exists rather than by showing it, and it is the
// common case whenever the directory on screen was just deleted. pkg/sftp normalises
// SSH_FX_NO_SUCH_FILE to os.ErrNotExist, which is what makes the check possible here.
func listError(dirPath string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return domain.ErrDirectoryNotFound
	}
	return fmt.Errorf("sftp list %s: %w", dirPath, err)
}
