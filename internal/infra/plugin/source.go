package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/infra/portable"
)

// LoadPluginSource loads a plugin directory or .xqsp bundle.
func LoadPluginSource(path string) (domainplugin.InstalledPlugin, error) {
	res, err := loadSource(path)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	defer res.cleanup()
	return res.plugin, nil
}

// InstallFromSource installs a plugin from a directory or .xqsp bundle.
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
	if err := CopyBundle(sourceDir, destDir, plugin.Manifest.Engine.Entry); err != nil {
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

// loadSource is the one gate both install routes pass through: a local .xqsp the user picked, and
// the staging directory a GitHub install prepared. Startup discovery does not come this way, which
// is what lets the checks here refuse a plugin without making an installed one unloadable.
func loadSource(path string) (loadedSource, error) {
	path = filepath.Clean(path)

	var (
		res loadedSource
		err error
	)
	if bundle.IsBundlePath(path) {
		res, err = loadBundleSource(path)
	} else {
		res, err = loadDirSource(path)
	}
	if err != nil {
		return loadedSource{}, err
	}

	if err := bundle.ValidateDeclaredUIAssets(&res.plugin.Manifest, res.plugin.RootDir); err != nil {
		res.cleanup()
		return loadedSource{}, err
	}
	return res, nil
}

func loadBundleSource(path string) (loadedSource, error) {
	tempBase := portable.Default.TempDir()
	if err := os.MkdirAll(tempBase, 0o700); err != nil {
		return loadedSource{}, fmt.Errorf("create portable temp dir: %w", err)
	}
	tempDir, err := os.MkdirTemp(tempBase, "xqsp-*")
	if err != nil {
		return loadedSource{}, err
	}
	if err := bundle.Extract(path, tempDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return loadedSource{}, fmt.Errorf("extract bundle: %w", err)
	}
	if err := bundle.RequireChecksums(tempDir, InstallMetaFile, UserInstalledMarker); err != nil {
		_ = os.RemoveAll(tempDir)
		return loadedSource{}, fmt.Errorf("validate checksums: %w", err)
	}
	plugin, err := LoadPluginDir(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return loadedSource{}, err
	}
	// ChecksumsDigest is captured while tempDir still exists (before cleanup below).
	return loadedSource{plugin: plugin, tempDir: tempDir}, nil
}

func loadDirSource(path string) (loadedSource, error) {
	plugin, err := LoadPluginDir(path)
	if err != nil {
		return loadedSource{}, err
	}
	if err := bundle.ValidateChecksums(path, InstallMetaFile, UserInstalledMarker); err != nil {
		return loadedSource{}, err
	}
	return loadedSource{plugin: plugin}, nil
}
