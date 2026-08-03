package pathsafe

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveHostPath normalizes path for trusted host operations without a sandbox root.
//
// Security: see ADR-007 (docs/adr/007-host-filesystem-trust.md). Callers are host UI
// only; plugin code must use SecurePathUnderRoots via FSProxy.
func ResolveHostPath(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", ErrPathDenied
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", ErrPathDenied
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve host path: %w", err)
	}
	return filepath.Clean(abs), nil
}
