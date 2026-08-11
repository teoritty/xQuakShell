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
	if err := validateInstalledTree(destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return domainplugin.InstalledPlugin{}, err
	}
	installed, err := LoadPluginDir(destDir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed.Source = domainplugin.SourceUser
	return installed, nil
}

// validateInstalledTree re-hashes the tree that will actually run and checks it against the
// SHA256SUMS that landed with it.
//
// This is deliberately not the same statement as the validation loadSource already performed. That
// one described the source tree, which for both install routes is a staging directory under the
// portable temp root — the directory every running plugin receives as its TEMP (process_env.go).
// A plugin that swapped the entry binary between that validation and CopyBundle would otherwise be
// installed under the victim plugin's id, inheriting its identity and the consents already granted
// to it, and the existence check on SHA256SUMS this replaced could not tell the difference.
//
// The reserved names are the two files the host itself writes into an installed tree; they are
// legitimately absent from the author's SHA256SUMS, and omitting either here fails every install.
func validateInstalledTree(destDir string) error {
	if err := bundle.ValidateChecksums(destDir, InstallMetaFile, UserInstalledMarker); err != nil {
		return fmt.Errorf("validate installed plugin: %w", err)
	}
	return nil
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
