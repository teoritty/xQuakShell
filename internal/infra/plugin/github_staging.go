package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

// StageGitHubPlugin prepares a plugin directory from a downloaded binary and manifest.
func StageGitHubPlugin(binaryPath string, manifest domainplugin.Manifest) (string, error) {
	tempDir, err := os.MkdirTemp("", "xqs-github-stage-*")
	if err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	entryName := manifest.Engine.Entry
	destBinary := filepath.Join(tempDir, entryName)
	if err := copyFileTo(binaryPath, destBinary); err != nil {
		cleanup()
		return "", fmt.Errorf("copy binary: %w", err)
	}
	if err := os.Chmod(destBinary, 0o700); err != nil {
		cleanup()
		return "", err
	}

	manifestPath := filepath.Join(tempDir, "plugin.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		cleanup()
		return "", err
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		cleanup()
		return "", err
	}

	if err := bundle.WriteChecksums(tempDir); err != nil {
		cleanup()
		return "", fmt.Errorf("write checksums: %w", err)
	}

	return tempDir, nil
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
