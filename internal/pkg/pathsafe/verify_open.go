package pathsafe

import (
	"fmt"
	"os"
	"path/filepath"
)

// VerifyOpenFileUnderRoots re-validates an open descriptor against allowed roots.
func VerifyOpenFileUnderRoots(f *os.File, roots []string) error {
	if f == nil {
		return ErrPathDenied
	}
	final, err := FinalPath(f)
	if err != nil {
		return fmt.Errorf("verify open file: %w", err)
	}
	final = filepath.Clean(final)
	if _, err := SecurePathUnderRoots(final, roots); err != nil {
		return ErrPathDenied
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrPathDenied
	}
	return nil
}
