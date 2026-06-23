package plugin

import (
	"fmt"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/pathsafe"
)

// SafePluginInstallDir returns an absolute install directory under dataRoot/plugins.
func SafePluginInstallDir(dataRoot, pluginID string) (string, error) {
	if err := domainplugin.ValidateID(pluginID); err != nil {
		return "", err
	}
	pluginsRoot, err := filepath.Abs(filepath.Join(dataRoot, "plugins"))
	if err != nil {
		return "", fmt.Errorf("resolve plugins root: %w", err)
	}
	dest, err := filepath.Abs(filepath.Join(pluginsRoot, pluginID))
	if err != nil {
		return "", fmt.Errorf("resolve install dir: %w", err)
	}
	if !pathsafe.UnderRoot(pluginsRoot, dest) {
		return "", fmt.Errorf("%w: install path escapes plugins root", domainplugin.ErrInvalidManifest)
	}
	return dest, nil
}
