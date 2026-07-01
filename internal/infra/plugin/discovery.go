package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

const (
	// UserInstalledMarker marks plugins installed via the UI (vs shipped in exeDir/plugins).
	UserInstalledMarker = ".xqs-user-installed"
)

type Discovery struct {
	searchPaths []string
}

// NewDiscovery creates a discovery service for the given search paths (in priority order).
func NewDiscovery(searchPaths []string) *Discovery {
	return &Discovery{searchPaths: searchPaths}
}

// Discover loads all valid plugins. Later search paths override earlier IDs with the same manifest id.
func (d *Discovery) Discover() ([]domainplugin.InstalledPlugin, error) {
	byID := make(map[string]domainplugin.InstalledPlugin)

	for _, root := range d.searchPaths {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read plugins dir %s: %w", root, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginDir := filepath.Join(root, entry.Name())
			installed, err := loadPluginDir(pluginDir)
			if err != nil {
				slog.Warn("skip plugin load", "dir", pluginDir, "err", err)
				continue
			}
			if existing, ok := byID[installed.Manifest.ID]; ok {
				if existing.Source == domainplugin.SourceUser && installed.Source == domainplugin.SourceBundled {
					continue
				}
			}
			byID[installed.Manifest.ID] = installed
		}
	}

	result := make([]domainplugin.InstalledPlugin, 0, len(byID))
	for _, p := range byID {
		result = append(result, p)
	}
	return result, nil
}

// LoadPluginDir validates and loads a plugin directory (install preview).
func LoadPluginDir(dir string) (domainplugin.InstalledPlugin, error) {
	return loadPluginDir(dir)
}

func loadPluginDir(dir string) (domainplugin.InstalledPlugin, error) {
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}

	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("decode plugin.json: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if err := bundle.ValidateCapabilitiesForInstall(&manifest, dir); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}

	source := detectInstallSource(dir)
	if err := verifyPluginIntegrity(dir, manifest); err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("plugin integrity check failed: %w", err)
	}

	if manifest.Signature != "" {
		if _, err := os.Stat(filepath.Join(dir, bundle.ChecksumsFile)); os.IsNotExist(err) {
			return domainplugin.InstalledPlugin{}, fmt.Errorf("%w: signed plugin requires SHA256SUMS", domainplugin.ErrInvalidManifest)
		}
	}

	checksumsDigest, err := bundle.ChecksumsDigest(dir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("checksums digest: %w", err)
	}

	entryPath, err := ResolveEngineEntryPath(dir, manifest.Engine.Entry)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if _, err := os.Stat(entryPath); err != nil {
		if alt := resolveEntryAlternate(dir, manifest.Engine.Entry); alt != "" {
			altPath, altErr := ResolveEngineEntryPath(dir, filepath.Base(alt))
			if altErr != nil {
				return domainplugin.InstalledPlugin{}, altErr
			}
			if _, statErr := os.Stat(altPath); statErr != nil {
				return domainplugin.InstalledPlugin{}, fmt.Errorf("plugin binary missing: %w", statErr)
			}
			manifest.Engine.Entry = filepath.Base(alt)
		} else {
			return domainplugin.InstalledPlugin{}, fmt.Errorf("plugin binary missing: %w", err)
		}
	}

	return domainplugin.InstalledPlugin{
		Manifest:        manifest,
		RootDir:         dir,
		Source:          source,
		ChecksumsDigest: checksumsDigest,
	}, nil
}

func verifyPluginIntegrity(dir string, manifest domainplugin.Manifest) error {
	if manifest.Signature != "" {
		return bundle.RequireChecksums(dir)
	}
	if bundle.HasChecksums(dir) {
		return bundle.ValidateChecksums(dir)
	}
	return nil
}

// SearchPaths returns plugin directories in priority order (ADR-006).
//
// Writable user state lives under dataRoot/plugins. Bundled reference plugins may ship read-only
// next to the executable at exeDir/plugins; they must include SHA256SUMS and never override
// a user-installed copy with the same manifest id.
func SearchPaths(exeDir, dataRoot string) []string {
	paths := make([]string, 0, 2)
	if dataRoot != "" {
		paths = append(paths, filepath.Join(dataRoot, "plugins"))
	}
	if exeDir != "" {
		paths = append(paths, filepath.Join(exeDir, "plugins"))
	}
	return paths
}

// PluginDataDir returns the writable data directory for a plugin instance.
func PluginDataDir(dataRoot, pluginID string) string {
	safeID := strings.ReplaceAll(pluginID, string(filepath.Separator), "_")
	return filepath.Join(dataRoot, "plugins", safeID, "data")
}

func detectInstallSource(dir string) domainplugin.InstallSource {
	if _, err := os.Stat(filepath.Join(dir, UserInstalledMarker)); err == nil {
		return domainplugin.SourceUser
	}
	return domainplugin.SourceBundled
}

// MarkUserInstalled records that a plugin was installed via the UI.
func MarkUserInstalled(pluginDir string) error {
	return os.WriteFile(filepath.Join(pluginDir, UserInstalledMarker), nil, 0644)
}

func resolveEntryAlternate(dir, entry string) string {
	candidates := []string{}
	if strings.HasSuffix(strings.ToLower(entry), ".exe") {
		candidates = append(candidates, strings.TrimSuffix(entry, ".exe"))
	} else {
		candidates = append(candidates, entry+".exe")
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
