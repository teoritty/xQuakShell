package plugin

import (
	"fmt"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/pathsafe"
)

// ResolveEngineEntryPath resolves engine.entry under rootDir with prefix and symlink checks.
func ResolveEngineEntryPath(rootDir, entry string) (string, error) {
	if err := domainplugin.ValidateBundleRelativePath(entry); err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve plugin root: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)

	candidate, err := pathsafe.ResolveUnderRoot(rootAbs, entry)
	if err != nil {
		return "", fmt.Errorf("%w: engine.entry escapes plugin bundle", domainplugin.ErrInvalidManifest)
	}
	resolved, err := pathsafe.SecurePathUnderRoots(candidate, []string{rootAbs})
	if err != nil {
		return "", fmt.Errorf("%w: engine.entry escapes plugin bundle", domainplugin.ErrInvalidManifest)
	}
	return resolved, nil
}
