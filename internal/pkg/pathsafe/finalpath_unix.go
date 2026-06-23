//go:build unix

package pathsafe

import (
	"fmt"
	"os"
	"path/filepath"
)

// FinalPath returns the absolute path of an open file descriptor.
func FinalPath(f *os.File) (string, error) {
	if f == nil {
		return "", fmt.Errorf("nil file")
	}
	return filepath.EvalSymlinks("/proc/self/fd/" + fmt.Sprint(int(f.Fd())))
}
