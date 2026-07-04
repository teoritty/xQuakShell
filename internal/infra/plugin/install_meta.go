package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
)

const InstallMetaFile = "install-meta.json"

// WriteInstallMeta persists install provenance next to plugin.json.
func WriteInstallMeta(pluginDir string, meta domainplugin.PluginInstallMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install meta: %w", err)
	}
	path := filepath.Join(pluginDir, InstallMetaFile)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write install meta: %w", err)
	}
	return nil
}

// LoadInstallMeta reads optional install provenance from a plugin directory.
func LoadInstallMeta(pluginDir string) (domainplugin.PluginInstallMeta, bool, error) {
	path := filepath.Join(pluginDir, InstallMetaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domainplugin.PluginInstallMeta{}, false, nil
		}
		return domainplugin.PluginInstallMeta{}, false, err
	}
	var meta domainplugin.PluginInstallMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return domainplugin.PluginInstallMeta{}, false, fmt.Errorf("decode install meta: %w", err)
	}
	return meta, true, nil
}
