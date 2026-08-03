package capability

import (
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

// SecurePathUnderRoots resolves absPath and verifies it stays within roots without symlink escape.
func SecurePathUnderRoots(absPath string, roots []string) (string, error) {
	resolved, err := pathsafe.SecurePathUnderRoots(absPath, roots)
	if err != nil {
		if err == pathsafe.ErrPathDenied {
			return "", domainplugin.ErrCapabilityDenied
		}
		return "", err
	}
	return resolved, nil
}
