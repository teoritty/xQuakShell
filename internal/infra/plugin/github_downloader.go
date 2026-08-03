package plugin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	infragithub "xquakshell/internal/infra/github"
	"xquakshell/internal/pkg/pathsafe"
)

// ErrReleaseTooLarge is returned when a downloaded release asset, or the archive it
// expands to, exceeds the domainplugin.MaxRelease* limits.
var ErrReleaseTooLarge = fmt.Errorf("release asset too large")

// errSkipEntry marks an archive entry that is harmless but has nothing to extract
// (the archive-root "./" entry). Callers skip it; it is never a failure.
var errSkipEntry = errors.New("archive entry skipped")

// releaseBudget bounds one extraction run: entry count, per-entry size, and running
// total. It mirrors the extractBudget used for .xqsp bundles — same threat (a zip/tar
// bomb from a release we do not control), different limits.
type releaseBudget struct {
	entries int
	bytes   int64
}

// addEntry charges an entry of the given uncompressed size against the budget. The size
// comes from the archive header, which the archive author controls, so it is only a
// first gate; the copy itself is separately bounded by copyBounded.
func (b *releaseBudget) addEntry(uncompressed int64) error {
	b.entries++
	if b.entries > domainplugin.MaxReleaseEntryCount {
		return fmt.Errorf("%w: too many entries", ErrReleaseTooLarge)
	}
	if uncompressed < 0 {
		return fmt.Errorf("%w: invalid entry size", ErrReleaseTooLarge)
	}
	if uncompressed > domainplugin.MaxReleaseEntryBytes {
		return fmt.Errorf("%w: entry exceeds %d bytes", ErrReleaseTooLarge, domainplugin.MaxReleaseEntryBytes)
	}
	b.bytes += uncompressed
	if b.bytes > domainplugin.MaxReleaseUncompressedBytes {
		return fmt.Errorf("%w: total uncompressed size exceeds %d bytes", ErrReleaseTooLarge, domainplugin.MaxReleaseUncompressedBytes)
	}
	return nil
}

// copyBounded copies at most limit bytes from src to dst and fails if src has more,
// so a lying archive header cannot buy an unbounded write.
func copyBounded(dst io.Writer, src io.Reader, limit int64) error {
	n, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("%w: entry exceeds %d bytes", ErrReleaseTooLarge, limit)
	}
	return nil
}

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

	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}

	budget := releaseBudget{}
	for _, file := range reader.File {
		path, err := safeExtractPath(destDir, file.Name)
		if errors.Is(err, errSkipEntry) {
			continue
		}
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
			continue
		}
		// UncompressedSize64 is attacker-controlled metadata; charging it first rejects
		// an obvious bomb cheaply, and copyBounded below catches a header that lies.
		// #nosec G115 -- a uint64 above MaxInt64 wraps to a negative, which addEntry rejects.
		if err := budget.addEntry(int64(file.UncompressedSize64)); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
		err = copyBounded(dstFile, srcFile, domainplugin.MaxReleaseEntryBytes)
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
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}

	budget := releaseBudget{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path, err := safeExtractPath(destDir, header.Name)
		if errors.Is(err, errSkipEntry) {
			continue
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			// header.Size is attacker-controlled; copyBounded below is the real bound.
			if err := budget.addEntry(header.Size); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			// #nosec G115 -- header.Mode is masked to permission bits by os.FileMode below.
			mode := os.FileMode(header.Mode&0o777) | 0o100
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if err := copyBounded(out, tr, domainplugin.MaxReleaseEntryBytes); err != nil {
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

// safeExtractPath resolves an archive entry name to an absolute path inside destDir,
// or fails. It mirrors the containment logic in plugin/bundle.extractZipFile: reject
// the name outright when it is absolute or walks up, then verify the *resolved*
// path — the value the caller actually opens — still sits under the destination.
//
// Checking only the entry name, or only an intermediate like filepath.Rel's result,
// is what lets a zip-slip through: the guard has to hold on the string handed to
// os.OpenFile/os.MkdirAll.
func safeExtractPath(destDir, name string) (string, error) {
	invalid := fmt.Errorf("invalid archive path: %s", name)

	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." {
		// `tar -czf x.tgz .` emits a "./" entry for the archive root. It is not an
		// escape and rejecting it would fail otherwise-valid release tarballs, but
		// it resolves to destDir itself, which the containment check below (rightly)
		// treats as "not under destDir". Skip it instead.
		return "", errSkipEntry
	}
	if filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) ||
		strings.Contains(clean, string(filepath.Separator)+"..") {
		return "", invalid
	}
	// Two Windows-only shapes that filepath.IsAbs does not catch, both of which
	// are absolute on the platform the archive was most likely built on:
	//   "C:foo"       - volume-relative
	//   "\etc\passwd" - root-relative (a POSIX "/etc/passwd" after FromSlash)
	if filepath.VolumeName(clean) != "" || strings.HasPrefix(clean, string(filepath.Separator)) {
		return "", invalid
	}

	root, err := filepath.Abs(destDir)
	if err != nil {
		return "", fmt.Errorf("resolve dest dir: %w", err)
	}
	root = filepath.Clean(root)

	full := filepath.Clean(filepath.Join(root, clean))
	if !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", invalid
	}
	if !pathsafe.UnderRoot(root, full) {
		return "", invalid
	}
	return full, nil
}
