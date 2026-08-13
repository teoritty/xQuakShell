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
	if req.ExpectedChecksum == "" {
		if !req.AllowUnverified {
			return "", fmt.Errorf("%w: release %s lists no SHA256SUMS entry for %s",
				domainplugin.ErrChecksumUnavailable, req.Tag, req.AssetName)
		}
		return tempFile, nil
	}
	if !strings.EqualFold(actualChecksum, req.ExpectedChecksum) {
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
		// SHA256SUMS is the one asset with nothing to verify itself against; it is the listing.
		AllowUnverified: true,
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

// findEntryExecutable resolves the manifest's engine.entry to a file inside an extracted release
// archive, as a path.
//
// Selecting it by mode bits, as this once did, does not work: both extractors force the execute
// bit onto every entry — they have to, because an archive written on Windows carries none — so
// "the first file that looks executable" is really "the first file in walk order", which installs
// a README as the plugin and only fails much later, at spawn.
//
// Searching for the base name anywhere in the tree, which is what replaced it, is worse. It threw
// away the path the manifest declares and kept only the last segment, then took whichever match
// filepath.Walk reached first — so an archive carrying both a/plug and bin/plug installs a/plug,
// because "a" sorts before "bin". Anyone who can write the archive (its author, or a man in the
// middle on a release with no SHA256SUMS) plants a decoy beside the real binary and the decoy is
// what gets the execute bit and gets spawned. It also meant this path applied none of the
// containment ResolveEngineEntryPath applies everywhere else.
//
// So the entry is resolved as what it is: a relative path under the archive root. The single
// concession is the wrapper directory - `tar czf x.tgz myplugin-1.2.0/` is how release tarballs
// are usually built, and the manifest path is relative to the plugin, not to that wrapper. It is
// only tried when the root holds exactly one entry and that entry is a directory, so it can never
// become a search: two candidates mean the archive is not the shape this understands, and
// guessing between them is how the decoy got in.
func findEntryExecutable(dir, entryName, assetName string) (string, error) {
	if strings.TrimSpace(entryName) == "" {
		return "", fmt.Errorf("cannot pick a binary out of release asset %q: the manifest declares no engine.entry", assetName)
	}

	roots := []string{dir}
	if wrapper, ok := soleWrapperDir(dir); ok {
		roots = append(roots, wrapper)
	}

	for _, root := range roots {
		for _, candidate := range entryNameCandidates(entryName) {
			resolved, err := ResolveEngineEntryPath(root, candidate)
			if err != nil {
				// A traversing or absolute engine.entry is a property of the manifest, not of the
				// root being tried, so it fails the install rather than moving on to the next one.
				return "", err
			}
			if info, statErr := os.Stat(resolved); statErr == nil && !info.IsDir() {
				return resolved, nil
			}
		}
	}
	return "", fmt.Errorf("release asset %q does not contain %q at the path its manifest declares", assetName, entryName)
}

// soleWrapperDir returns the one directory an archive root contains, when that is all it contains.
func soleWrapperDir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return "", false
	}
	return filepath.Join(dir, entries[0].Name()), true
}
