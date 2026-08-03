package pathsafe

import (
	"errors"
	"os"
)

// ErrPathDenied indicates a path escapes its allowed root or uses symlinks.
var ErrPathDenied = errors.New("path access denied")

// IsNotExist reports whether err indicates a missing path during secure open.
func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
