package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

// NewGitHubPluginStager returns a stager that creates staging directories under tempBase.
// When tempBase is empty, os.TempDir() is used.
func NewGitHubPluginStager(tempBase string) func(domainplugin.DownloadedAsset, domainplugin.Manifest) (domainplugin.StagedPlugin, func(), error) {
	return func(asset domainplugin.DownloadedAsset, manifest domainplugin.Manifest) (domainplugin.StagedPlugin, func(), error) {
		return stageGitHubPlugin(tempBase, asset, manifest)
	}
}

func stageGitHubPlugin(tempBase string, asset domainplugin.DownloadedAsset, manifest domainplugin.Manifest) (domainplugin.StagedPlugin, func(), error) {
	noop := func() {}
	if tempBase == "" {
		tempBase = os.TempDir()
	}
	tempDir, err := os.MkdirTemp(tempBase, "xqs-github-stage-*")
	if err != nil {
		return domainplugin.StagedPlugin{}, noop, fmt.Errorf("create staging dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	var staged domainplugin.Manifest
	switch asset.Kind {
	case domainplugin.ReleaseAssetBundle:
		staged, err = stageBundle(tempDir, asset.Path)
	default:
		staged, err = stageBinary(tempDir, asset.Path, manifest)
	}
	if err != nil {
		cleanup()
		return domainplugin.StagedPlugin{}, noop, err
	}

	return domainplugin.StagedPlugin{Dir: tempDir, Manifest: staged}, cleanup, nil
}

// stageBinary builds a minimal plugin tree around a bare release binary: the binary itself, the
// manifest the repository published, and checksums over the two.
//
// Those checksums are a container, not a claim of authenticity — we compute them over files we
// just wrote ourselves, so validating them later proves only that nothing was corrupted in
// between. That is the whole reason a plugin which ships ui/ assets may not be installed this
// way: there would be no author statement about the assets, and no assets either.
func stageBinary(tempDir, binaryPath string, manifest domainplugin.Manifest) (domainplugin.Manifest, error) {
	destBinary := filepath.Join(tempDir, manifest.Engine.Entry)
	if err := copyFileTo(binaryPath, destBinary); err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("copy binary: %w", err)
	}
	// #nosec G302 -- the plugin entry point must be executable; 0700 is already the
	// narrowest mode that allows the host to exec it, and the staging dir is 0700.
	if err := os.Chmod(destBinary, 0o700); err != nil {
		return domainplugin.Manifest{}, err
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return domainplugin.Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "plugin.json"), data, 0o600); err != nil {
		return domainplugin.Manifest{}, err
	}

	if err := bundle.WriteChecksums(tempDir); err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("write checksums: %w", err)
	}
	return manifest, nil
}

func copyFileTo(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// #nosec G302 -- copyFileTo is only used to stage the plugin entry binary, which
	// must carry the execute bit; 0700 keeps it owner-only.
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
