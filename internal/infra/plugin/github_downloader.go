package plugin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	infragithub "ssh-client/internal/infra/github"
)

// BinaryDownloader downloads and verifies plugin binaries from GitHub Releases.
type BinaryDownloader struct {
	githubClient *infragithub.Client
	tempDir      string
}

// NewBinaryDownloader creates a new downloader.
func NewBinaryDownloader(githubClient *infragithub.Client) *BinaryDownloader {
	return &BinaryDownloader{
		githubClient: githubClient,
		tempDir:      os.TempDir(),
	}
}

// DownloadBinary downloads a plugin binary from GitHub Releases.
func (d *BinaryDownloader) DownloadBinary(
	ctx context.Context,
	owner, repo, tag, assetName, expectedChecksum string,
) (string, error) {
	tempDir, err := os.MkdirTemp(d.tempDir, "xqs-plugin-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	release, err := d.githubClient.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}

	asset, err := infragithub.FindAsset(release.Assets, assetName)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}

	reader, err := d.githubClient.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	defer reader.Close()

	tempFile := filepath.Join(tempDir, assetName)
	outFile, err := os.Create(tempFile)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	if _, err := io.Copy(writer, reader); err != nil {
		outFile.Close()
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	if err := outFile.Close(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "" && !strings.EqualFold(actualChecksum, expectedChecksum) {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	lower := strings.ToLower(assetName)
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		extractDir := filepath.Join(tempDir, "extracted")
		if err := d.extractArchive(tempFile, extractDir); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}
		executable, err := d.findExecutable(extractDir)
		if err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}
		return executable, nil
	}

	return tempFile, nil
}

func (d *BinaryDownloader) extractArchive(archivePath, destDir string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return d.extractZIP(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return d.extractTAR(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive format")
	}
}

func (d *BinaryDownloader) extractZIP(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for _, file := range reader.File {
		path, err := safeExtractPath(destDir, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		srcFile, err := file.Open()
		if err != nil {
			return err
		}
		mode := file.Mode() | 0o100
		dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			srcFile.Close()
			return err
		}
		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *BinaryDownloader) extractTAR(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path, err := safeExtractPath(destDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode) | 0o100
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
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

func safeExtractPath(destDir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid archive path: %s", name)
	}
	full := filepath.Join(destDir, clean)
	rel, err := filepath.Rel(destDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid archive path: %s", name)
	}
	return full, nil
}
