package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	infragithub "xquakshell/internal/infra/github"
)

// BinaryDownloader downloads and verifies plugin binaries from GitHub Releases.
type BinaryDownloader struct {
	githubClient *infragithub.Client
	tempDir      string
}

// NewBinaryDownloader creates a new downloader using tempBase for staging directories.
// When tempBase is empty, os.TempDir() is used.
func NewBinaryDownloader(githubClient *infragithub.Client, tempBase string) *BinaryDownloader {
	if tempBase == "" {
		tempBase = os.TempDir()
	}
	return &BinaryDownloader{
		githubClient: githubClient,
		tempDir:      tempBase,
	}
}

// DownloadBinary downloads a plugin binary from GitHub Releases.
func (d *BinaryDownloader) DownloadBinary(
	ctx context.Context,
	owner, repo, tag, assetName, expectedChecksum string,
) (string, func(), error) {
	noop := func() {}
	if d == nil || d.githubClient == nil {
		return "", noop, fmt.Errorf("plugin downloader unavailable")
	}

	tempDir, err := os.MkdirTemp(d.tempDir, "xqsp-*")
	if err != nil {
		return "", noop, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	release, err := d.githubClient.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		cleanup()
		return "", noop, err
	}

	asset, err := infragithub.FindAsset(release.Assets, assetName)
	if err != nil {
		cleanup()
		return "", noop, err
	}

	reader, err := d.githubClient.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		cleanup()
		return "", noop, err
	}
	defer reader.Close()

	tempFile := filepath.Join(tempDir, assetName)
	outFile, err := os.Create(tempFile)
	if err != nil {
		cleanup()
		return "", noop, err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	if err := copyBounded(writer, reader, domainplugin.MaxReleaseAssetBytes); err != nil {
		outFile.Close()
		cleanup()
		return "", noop, err
	}
	if err := outFile.Close(); err != nil {
		cleanup()
		return "", noop, err
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "" && !strings.EqualFold(actualChecksum, expectedChecksum) {
		cleanup()
		return "", noop, fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	lower := strings.ToLower(assetName)
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		extractDir := filepath.Join(tempDir, "extracted")
		if err := d.extractArchive(tempFile, extractDir); err != nil {
			cleanup()
			return "", noop, err
		}
		executable, err := d.findExecutable(extractDir)
		if err != nil {
			cleanup()
			return "", noop, err
		}
		return executable, cleanup, nil
	}

	return tempFile, cleanup, nil
}

// DownloadAssetContent downloads a release asset and returns its contents.
func (d *BinaryDownloader) DownloadAssetContent(
	ctx context.Context,
	owner, repo, tag, assetName string,
) ([]byte, error) {
	path, cleanup, err := d.DownloadBinary(ctx, owner, repo, tag, assetName, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return os.ReadFile(path)
}

func (d *BinaryDownloader) findExecutable(dir string) (string, error) {
	var executable string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		mode := info.Mode()
		if mode&0o111 != 0 || strings.HasSuffix(strings.ToLower(path), ".exe") {
			executable = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	if executable == "" {
		return "", fmt.Errorf("no executable found in archive")
	}
	return executable, nil
}
