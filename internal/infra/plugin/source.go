package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/bundle"
	"ssh-client/internal/infra/portable"
)

// LoadPluginSource loads a plugin directory or .xqs-plugin bundle.
func LoadPluginSource(path string) (domainplugin.InstalledPlugin, error) {
	res, err := loadSource(path)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	defer res.cleanup()
	return res.plugin, nil
}

// InstallFromSource installs a plugin from a directory or .xqs-plugin bundle.
func InstallFromSource(sourcePath, dataRoot string) (domainplugin.InstalledPlugin, error) {
	res, err := loadSource(sourcePath)
	if err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("load plugin: %w", err)
	}
	defer res.cleanup()

	sourceDir := res.plugin.RootDir
	plugin := res.plugin
	destDir, err := SafePluginInstallDir(dataRoot, plugin.Manifest.ID)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if err := os.RemoveAll(destDir); err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("prepare install dir: %w", err)
	}
	if err := CopyBundle(sourceDir, destDir); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if err := MarkUserInstalled(destDir); err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("mark user install: %w", err)
	}
	if !HasChecksumsFile(destDir) {
		_ = os.RemoveAll(destDir)
		return domainplugin.InstalledPlugin{}, fmt.Errorf("user-installed plugins must include %s", bundle.ChecksumsFile)
	}
	installed, err := LoadPluginDir(destDir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed.Source = domainplugin.SourceUser
	return installed, nil
}

// ValidatePluginSource validates a plugin directory or bundle without installing.
func ValidatePluginSource(path string) error {
	res, err := loadSource(path)
	if err != nil {
		return err
	}
	defer res.cleanup()
	return res.plugin.Manifest.Validate()
}

// HasChecksumsFile reports whether SHA256SUMS exists in a plugin tree.
func HasChecksumsFile(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, bundle.ChecksumsFile))
	return err == nil
}

type loadedSource struct {
	plugin  domainplugin.InstalledPlugin
	tempDir string
}

func (r loadedSource) cleanup() {
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
}

func loadSource(path string) (loadedSource, error) {
	path = filepath.Clean(path)
	if bundle.IsBundlePath(path) {
		tempBase := portable.Default.TempDir()
		if err := os.MkdirAll(tempBase, 0o700); err != nil {
			return loadedSource{}, fmt.Errorf("create portable temp dir: %w", err)
		}
		tempDir, err := os.MkdirTemp(tempBase, "xqs-plugin-*")
		if err != nil {
			return loadedSource{}, err
		}
		if err := bundle.Extract(path, tempDir); err != nil {
			_ = os.RemoveAll(tempDir)
			return loadedSource{}, fmt.Errorf("extract bundle: %w", err)
		}
		if err := bundle.RequireChecksums(tempDir); err != nil {
			_ = os.RemoveAll(tempDir)
			return loadedSource{}, fmt.Errorf("validate checksums: %w", err)
		}
		plugin, err := LoadPluginDir(tempDir)
		if err != nil {
			_ = os.RemoveAll(tempDir)
			return loadedSource{}, err
		}
		return loadedSource{plugin: plugin, tempDir: tempDir}, nil
	}

	plugin, err := LoadPluginDir(path)
	if err != nil {
		return loadedSource{}, err
	}
	if err := bundle.ValidateChecksums(path); err != nil {
		return loadedSource{}, err
	}
	return loadedSource{plugin: plugin}, nil
}
