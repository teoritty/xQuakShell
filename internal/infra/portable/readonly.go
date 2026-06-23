package portable

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// DetectReadOnlyDataRoot reports whether the portable data root is read-only.
func DetectReadOnlyDataRoot(p *Paths) (bool, error) {
	if p == nil {
		return false, nil
	}
	probe := filepath.Join(p.DataRoot(), ".write-probe")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
		if errors.Is(err, syscall.EROFS) || os.IsPermission(err) {
			return true, nil
		}
		return false, err
	}
	_ = os.Remove(probe)
	return false, nil
}
