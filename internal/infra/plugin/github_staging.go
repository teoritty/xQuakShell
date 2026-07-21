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
func NewGitHubPluginStager(tempBase string) func(string, domainplugin.Manifest) (string, func(), error) {
	return func(binaryPath string, manifest domainplugin.Manifest) (string, func(), error) {
		return stageGitHubPlugin(tempBase, binaryPath, manifest)
	}
}

func stageGitHubPlugin(tempBase string, binaryPath string, manifest domainplugin.Manifest) (string, func(), error) {
	noop := func() {}
	if tempBase == "" {
		tempBase = os.TempDir()
	}
	tempDir, err := os.MkdirTemp(tempBase, "xqs-github-stage-*")
	if err != nil {
		return "", noop, fmt.Errorf("create staging dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	entryName := manifest.Engine.Entry
	destBinary := filepath.Join(tempDir, entryName)
	if err := copyFileTo(binaryPath, destBinary); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("copy binary: %w", err)
	}
	if err := os.Chmod(destBinary, 0o700); err != nil {
		cleanup()
		return "", noop, err
	}

	manifestPath := filepath.Join(tempDir, "plugin.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		cleanup()
		return "", noop, err
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		cleanup()
		return "", noop, err
	}

	if err := bundle.WriteChecksums(tempDir); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("write checksums: %w", err)
	}

	return tempDir, cleanup, nil
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

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
