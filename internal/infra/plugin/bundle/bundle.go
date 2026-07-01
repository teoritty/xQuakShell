package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/pathsafe"
)

const (
	// Extension is the plugin bundle file suffix.
	Extension = ".xqs-plugin"
	// ChecksumsFile lists SHA-256 hashes of bundle contents.
	ChecksumsFile = "SHA256SUMS"
)

// ErrMissingChecksums indicates a bundle or plugin tree lacks SHA256SUMS.
var ErrMissingChecksums = errors.New("bundle missing SHA256SUMS")

// ErrBundleTooLarge indicates a bundle exceeded safe extraction limits.
var ErrBundleTooLarge = errors.New("bundle exceeds safe size limits")

// IsBundlePath reports whether path looks like a plugin bundle archive.
func IsBundlePath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), Extension)
}

// HasChecksums reports whether SHA256SUMS exists in a plugin tree.
func HasChecksums(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ChecksumsFile))
	return err == nil
}

// BundleAdapter implements domainplugin.BundlePort.
type BundleAdapter struct{}

// HasChecksums implements domainplugin.BundlePort.
func (BundleAdapter) HasChecksums(dir string) bool {
	return HasChecksums(dir)
}

type fileHash struct {
	name string
	hash string
}

// Pack creates a .xqs-plugin zip from sourceDir including SHA256SUMS.
func Pack(sourceDir, outPath string) error {
	sourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return err
	}
	hashes, err := hashTree(sourceDir)
	if err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	for _, h := range hashes {
		src := filepath.Join(sourceDir, filepath.FromSlash(h.name))
		if err := addZipFile(zw, src, h.name); err != nil {
			return err
		}
	}

	sumContent := renderChecksums(hashes)
	if err := addZipBytes(zw, ChecksumsFile, []byte(sumContent)); err != nil {
		return err
	}
	return zw.Close()
}

// Extract unpacks a .xqs-plugin archive into destDir with zip-slip and zip-bomb guards.
func Extract(bundlePath, destDir string) error {
	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		return fmt.Errorf("open bundle: %w", err)
	}
	defer r.Close()

	if len(r.File) > domainplugin.MaxBundleEntryCount {
		return fmt.Errorf("%w: too many entries (%d)", ErrBundleTooLarge, len(r.File))
	}

	if err := os.MkdirAll(destDir, 0700); err != nil {
		return err
	}

	budget := extractBudget{}
	for _, f := range r.File {
		if err := extractZipFile(f, destDir, &budget); err != nil {
			return err
		}
	}
	return nil
}

type extractBudget struct {
	entries int
	bytes   int64
}

func (b *extractBudget) addEntry(uncompressed int64) error {
	b.entries++
	if b.entries > domainplugin.MaxBundleEntryCount {
		return fmt.Errorf("%w: too many entries", ErrBundleTooLarge)
	}
	if uncompressed < 0 {
		return fmt.Errorf("%w: invalid entry size", ErrBundleTooLarge)
	}
	if uncompressed > domainplugin.MaxBundleEntryUncompressedBytes {
		return fmt.Errorf("%w: entry exceeds %d bytes", ErrBundleTooLarge, domainplugin.MaxBundleEntryUncompressedBytes)
	}
	b.bytes += uncompressed
	if b.bytes > domainplugin.MaxBundleUncompressedBytes {
		return fmt.Errorf("%w: total uncompressed size exceeds %d bytes", ErrBundleTooLarge, domainplugin.MaxBundleUncompressedBytes)
	}
	return nil
}

// normalizeLineEndings replaces CRLF with LF for cross-platform consistency.
func normalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// ChecksumsDigest returns the hex-encoded SHA-256 digest of the SHA256SUMS file in dir.
// Line endings are normalized (CRLF → LF) before hashing for cross-platform consistency.
// Returns ("", nil) if the file does not exist (legitimate for unsigned plugins).
func ChecksumsDigest(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ChecksumsFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	normalized := normalizeLineEndings(data)
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}

// WriteChecksums generates SHA256SUMS for all files in dir (except the checksums file itself).
// The file is always written with LF line endings, regardless of OS.
func WriteChecksums(dir string) error {
	hashes, err := hashTree(dir)
	if err != nil {
		return err
	}
	content := renderChecksums(hashes)
	sumPath := filepath.Join(dir, ChecksumsFile)
	return os.WriteFile(sumPath, []byte(content), 0o600)
}

// ValidateChecksums verifies SHA256SUMS exists and matches bundle contents.
func ValidateChecksums(dir string) error {
	sumPath := filepath.Join(dir, ChecksumsFile)
	data, err := os.ReadFile(sumPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w", ErrMissingChecksums)
		}
		return err
	}

	expected := parseChecksums(string(data))
	for name, want := range expected {
		path := filepath.Join(dir, filepath.FromSlash(name))
		got, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("checksum target %s: %w", name, err)
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return nil
}

// RequireChecksums verifies SHA256SUMS exists and matches bundle contents.
func RequireChecksums(dir string) error {
	sumPath := filepath.Join(dir, ChecksumsFile)
	if _, err := os.Stat(sumPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w", ErrMissingChecksums)
		}
		return err
	}
	return ValidateChecksums(dir)
}

func hashTree(root string) ([]fileHash, error) {
	var files []fileHash
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ChecksumsFile {
			return nil
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		files = append(files, fileHash{name: rel, hash: sum})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func renderChecksums(hashes []fileHash) string {
	var b strings.Builder
	for _, h := range hashes {
		b.WriteString(h.hash)
		b.WriteString("  ")
		b.WriteString(h.name)
		b.WriteByte('\n')
	}
	return b.String()
}

func parseChecksums(content string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		out[parts[1]] = parts[0]
	}
	return out
}

func addZipFile(zw *zip.Writer, srcPath, name string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(name)
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(w, in)
	return err
}

func addZipBytes(zw *zip.Writer, name string, data []byte) error {
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func extractZipFile(f *zip.File, destDir string, budget *extractBudget) error {
	uncompressed := int64(f.UncompressedSize64)
	if err := budget.addEntry(uncompressed); err != nil {
		return err
	}

	name := filepath.Clean(filepath.FromSlash(f.Name))
	if filepath.IsAbs(name) {
		return fmt.Errorf("invalid zip entry (absolute path): %q", f.Name)
	}
	if strings.HasPrefix(name, "..") || strings.Contains(name, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid zip entry %q", f.Name)
	}

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve dest dir: %w", err)
	}
	target := filepath.Join(absDest, name)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve zip target: %w", err)
	}
	if !pathsafe.UnderRoot(absDest, absTarget) {
		return fmt.Errorf("zip entry escapes destination: %q", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(absTarget, 0700)
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0700); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(absTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	limited := io.LimitReader(rc, domainplugin.MaxBundleEntryUncompressedBytes+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		return err
	}
	if written > domainplugin.MaxBundleEntryUncompressedBytes {
		return fmt.Errorf("%w: entry exceeds %d bytes", ErrBundleTooLarge, domainplugin.MaxBundleEntryUncompressedBytes)
	}
	if uncompressed > 0 && written != uncompressed {
		return fmt.Errorf("%w: entry size mismatch", ErrBundleTooLarge)
	}
	return out.Close()
}
