package assets

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

// DiscoveryIconReader reads a plugin's declared discovery icon assets from disk and hands them back
// as data URIs, keyed by icon ID (ADR-014 §manifest).
//
// It exists so that the frontend never learns a path. The tree addresses icons by ID, the bytes
// travel inline with ListPlugins, and no endpoint has to be defended against a plugin ID plus a
// crafted asset name. Rendering is <img src="data:..."> exclusively — inline SVG would execute the
// plugin bundle's scripts inside the main window (ADR-014 §Security model).
type DiscoveryIconReader struct{}

// ReadDiscoveryIcons returns iconID -> data URI for every asset it could read.
//
// A missing, unreadable, or empty asset is skipped rather than propagated: an icon is decoration,
// and refusing to load a plugin — or failing ListPlugins, which would blank the whole plugin list —
// over one unreadable glyph would cost the user everything the plugin does for the sake of a
// picture. The on-disk size budgets (64 KiB per asset, 1 MiB total) are enforced once at load and
// install time by bundle.ValidateCapabilitiesForInstall and are deliberately not re-checked here.
//
// Every skipped asset is reported in ONE log line per plugin. Per-asset lines would multiply by the
// 64-icon ceiling, and a line per render would turn a static packaging mistake into a log flood
// paced by the user's clicking.
func (DiscoveryIconReader) ReadDiscoveryIcons(p domainplugin.InstalledPlugin) map[string]string {
	icons := p.Manifest.Contributions.DiscoveryIcons
	if len(icons) == 0 {
		return nil
	}
	result := make(map[string]string, len(icons))
	var skipped []string
	for _, icon := range icons {
		uri, ok := readDiscoveryIcon(p.RootDir, icon.Asset)
		if !ok {
			skipped = append(skipped, icon.ID)
			continue
		}
		result[icon.ID] = uri
	}
	if len(skipped) > 0 {
		slog.Warn("plugin discovery icons could not be read; nodes will render without them",
			"component", "plugin.assets", "pluginId", p.Manifest.ID, "iconIds", strings.Join(skipped, " "))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// readDiscoveryIcon resolves one asset under the plugin's own root and encodes it.
//
// The containment check is a second lock on a door ValidateViewAssetEntry already bolted at install
// time. It is cheap, it runs once per icon per load, and it is the only thing standing between a
// manifest that reached the registry by some path this package cannot see and an os.ReadFile with a
// plugin-authored argument.
func readDiscoveryIcon(rootDir, asset string) (string, bool) {
	asset = strings.TrimSpace(asset)
	if rootDir == "" || asset == "" {
		return "", false
	}
	full := filepath.Join(rootDir, filepath.FromSlash(asset))
	if !pathsafe.UnderRoot(rootDir, full) {
		return "", false
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	return domainplugin.DiscoveryIconDataURI(asset, data)
}
