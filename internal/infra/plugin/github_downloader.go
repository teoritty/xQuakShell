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
// DownloadAsset downloads one release asset and reports what it turned out to be.
//
// A bundle asset is handed back as the downloaded file: unpacking it is the stager's job, and
// doing it here would leave the caller holding a path to one file out of a tree it needs whole.
// An archived binary is extracted and the plugin entry point picked out of it; anything else is
// the binary itself.
func (d *BinaryDownloader) DownloadAsset(
	ctx context.Context,
	req domainplugin.AssetDownloadRequest,
) (domainplugin.DownloadedAsset, func(), error) {
	noop := func() {}
	if d == nil || d.githubClient == nil {
		return domainplugin.DownloadedAsset{}, noop, fmt.Errorf("plugin downloader unavailable")
	}

	tempDir, err := os.MkdirTemp(d.tempDir, "xqsp-*")
	if err != nil {
		return domainplugin.DownloadedAsset{}, noop, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	fail := func(err error) (domainplugin.DownloadedAsset, func(), error) {
		cleanup()
		return domainplugin.DownloadedAsset{}, noop, err
	}

	tempFile, err := d.fetchAssetFile(ctx, tempDir, req)
	if err != nil {
		return fail(err)
	}

	kind := domainplugin.ClassifyReleaseAsset(req.AssetName)
	path := tempFile
	if kind == domainplugin.ReleaseAssetBinary && isSupportedArchive(req.AssetName) {
		extractDir := filepath.Join(tempDir, "extracted")
		if err := d.extractArchive(tempFile, extractDir); err != nil {
			return fail(err)
		}
		if path, err = findEntryExecutable(extractDir, req.EntryName, req.AssetName); err != nil {
			return fail(err)
		}
	}

	return domainplugin.DownloadedAsset{Path: path, Kind: kind, AssetName: req.AssetName}, cleanup, nil
}

// fetchAssetFile downloads the asset into tempDir and verifies it against the release-level
// SHA256SUMS entry, when the release published one.
func (d *BinaryDownloader) fetchAssetFile(ctx context.Context, tempDir string, req domainplugin.AssetDownloadRequest) (string, error) {
	release, err := d.githubClient.GetReleaseByTag(ctx, req.Owner, req.Repo, req.Tag)
	if err != nil {
		return "", err
	}

	asset, err := infragithub.FindAsset(release.Assets, req.AssetName)
	if err != nil {
		return "", err
	}

	reader, err := d.githubClient.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	tempFile := filepath.Join(tempDir, req.AssetName)
	outFile, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	if err := copyBounded(writer, reader, domainplugin.MaxReleaseAssetBytes); err != nil {
		outFile.Close()
		return "", err
	}
	if err := outFile.Close(); err != nil {
		return "", err
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if req.ExpectedChecksum != "" && !strings.EqualFold(actualChecksum, req.ExpectedChecksum) {
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", req.ExpectedChecksum, actualChecksum)
	}
	return tempFile, nil
}

// DownloadAssetContent downloads a release asset and returns its contents. It is used for the
// small text assets of a release (SHA256SUMS), which are never archives, so it needs no entry name.
func (d *BinaryDownloader) DownloadAssetContent(
	ctx context.Context,
	owner, repo, tag, assetName string,
) ([]byte, error) {
	asset, cleanup, err := d.DownloadAsset(ctx, domainplugin.AssetDownloadRequest{
		Owner:     owner,
		Repo:      repo,
		Tag:       tag,
		AssetName: assetName,
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return os.ReadFile(asset.Path)
}

func isSupportedArchive(assetName string) bool {
	lower := strings.ToLower(assetName)
	return strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz")
}

// findEntryExecutable picks the plugin binary out of an extracted release archive by name.
//
// Selecting it by mode bits, as this used to, does not work: both extractors force the execute
// bit onto every entry — they have to, because an archive written on Windows carries none — so
// "the first file that looks executable" is really "the first file in walk order", which
// installs a README or a licence as the plugin and only fails much later, at spawn. The manifest
// already states what the entry is called, and that is the only trustworthy answer here.
func findEntryExecutable(dir, entryName, assetName string) (string, error) {
	candidates := entryNameCandidates(entryName)
	if len(candidates) == 0 {
		return "", fmt.Errorf("cannot pick a binary out of release asset %q: the manifest declares no engine.entry", assetName)
	}
	wanted := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		wanted[strings.ToLower(filepath.Base(filepath.FromSlash(candidate)))] = struct{}{}
	}

	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if _, ok := wanted[strings.ToLower(info.Name())]; !ok {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("release asset %q contains no %s (looked for %v)", assetName, entryName, candidates)
	}
	return found, nil
}
