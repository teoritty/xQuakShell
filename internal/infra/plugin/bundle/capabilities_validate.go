package bundle

import (
	"path/filepath"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/pathsafe"
)

const placeholderPluginData = "${pluginData}"

// ValidateCapabilitiesForInstall validates manifest capabilities including on-disk FS roots.
func ValidateCapabilitiesForInstall(m *domainplugin.Manifest, installDir string) error {
	if err := m.ValidateCapabilities(); err != nil {
		return err
	}
	if installDir == "" || m.Capabilities.FS == nil {
		return nil
	}
	return validateFSRoots(m, installDir)
}

func validateFSRoots(m *domainplugin.Manifest, installDir string) error {
	installAbs, err := filepath.Abs(installDir)
	if err != nil {
		return err
	}
	pluginDataDir := filepath.Join(installAbs, "data")
	caps := m.Capabilities.FS
	patterns := append(append([]string{}, caps.Read...), caps.Write...)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		resolved := strings.ReplaceAll(pattern, placeholderPluginData, pluginDataDir)
		abs, err := filepath.Abs(resolved)
		if err != nil {
			return domainplugin.ErrInvalidManifest
		}
		abs = filepath.Clean(abs)
		if !pathsafe.UnderRoot(installAbs, abs) {
			return domainplugin.ErrInvalidManifest
		}
	}
	return nil
}
