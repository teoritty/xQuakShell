package bundle

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

const placeholderPluginData = "${pluginData}"

// ValidateCapabilitiesForInstall validates manifest capabilities including on-disk FS roots.
func ValidateCapabilitiesForInstall(m *domainplugin.Manifest, installDir string) error {
	if err := m.ValidateCapabilities(); err != nil {
		return err
	}
	if installDir == "" {
		return nil
	}
	if err := validateDiscoveryIconAssets(m, installDir); err != nil {
		return err
	}
	if m.Capabilities.FS == nil {
		return nil
	}
	return validateFSRoots(m, installDir)
}

// validateDiscoveryIconAssets enforces the ADR-014 on-disk icon budgets: 64 KiB per asset, 1 MiB
// across the plugin. It runs at load/install time and refuses the whole plugin, because the
// alternative — dropping the offending icon and running anyway — would leave a plugin silently
// missing part of its UI with nothing on screen to explain why.
//
// Every failure names the asset. "Plugin too large" sends an author hunting through 64 files; the
// path and the two byte counts point straight at the one to shrink.
//
// A missing file does not refuse the plugin: integrity verification owns that judgement, and
// reporting it here as a size problem would misdirect the reader. It is logged rather than passed
// over in silence, because integrity does not cover every asset of every unsigned bundled plugin —
// a manifest can be entirely valid while the icon it names is simply absent, and then the only
// symptom is a node that renders without an icon for no visible reason.
func validateDiscoveryIconAssets(m *domainplugin.Manifest, installDir string) error {
	icons := m.Contributions.DiscoveryIcons
	if len(icons) == 0 {
		return nil
	}
	var total int64
	for _, icon := range icons {
		// The path was confined to the bundle's ui/ tree by ValidateViewAssetEntry during
		// ValidateCapabilities above, which has already run by the time this is reached.
		rel := filepath.FromSlash(strings.TrimSpace(icon.Asset))
		info, err := os.Stat(filepath.Join(installDir, rel))
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("plugin declares a discovery icon whose asset is missing on disk",
					"component", "plugin.bundle", "pluginId", m.ID, "iconId", icon.ID, "asset", icon.Asset)
				continue
			}
			return fmt.Errorf("%w: discoveryIcons id %q: cannot read asset %q: %v",
				domainplugin.ErrInvalidManifest, icon.ID, icon.Asset, err)
		}
		size := info.Size()
		if size > domainplugin.MaxDiscoveryIconBytes {
			return fmt.Errorf("%w: discoveryIcons id %q: asset %q is %d bytes, over the %d byte per-icon limit",
				domainplugin.ErrInvalidManifest, icon.ID, icon.Asset, size, domainplugin.MaxDiscoveryIconBytes)
		}
		total += size
		if total > domainplugin.MaxDiscoveryIconTotalBytes {
			return fmt.Errorf("%w: discoveryIcons total size exceeds %d bytes at asset %q (id %q)",
				domainplugin.ErrInvalidManifest, domainplugin.MaxDiscoveryIconTotalBytes, icon.Asset, icon.ID)
		}
	}
	return nil
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
