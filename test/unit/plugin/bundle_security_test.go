package plugin_test

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/infra/plugin/bundle"
)

func TestBundleExtractRejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "evil.xqsp")

	zf, err := os.Create(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwn")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	if err := bundle.Extract(bundlePath, dest); err == nil {
		t.Fatal("expected zip-slip extraction to fail")
	}
}

func TestBundleExtractRejectsDotDotSegment(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "dots.xqsp")

	zf, err := os.Create(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("safe/../../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwn")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	if err := bundle.Extract(bundlePath, dest); err == nil {
		t.Fatal("expected dot-dot zip entry to fail")
	}
}

func TestBundleExtractRejectsMissingChecksumsOnRequire(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bundle.RequireChecksums(dest); err == nil {
		t.Fatal("expected missing checksums error")
	}
}

func TestValidateChecksumsRejectsUnlistedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}
	// Smuggle in a file that is not part of SHA256SUMS.
	if err := os.WriteFile(filepath.Join(dir, "evil.sh"), []byte("pwn"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bundle.ValidateChecksums(dir); err == nil {
		t.Fatal("expected unlisted file to be rejected")
	}
}

func TestValidateChecksumsAllowsReservedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}
	// Host-generated metadata written after packaging, not in SHA256SUMS.
	if err := os.WriteFile(filepath.Join(dir, "install-meta.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bundle.ValidateChecksums(dir); err == nil {
		t.Fatal("expected unlisted install-meta.json to be rejected without reserved arg")
	}
	if err := bundle.ValidateChecksums(dir, "install-meta.json"); err != nil {
		t.Fatalf("expected reserved file to be allowed: %v", err)
	}
}

func TestValidateChecksumsRejectsMissingListedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.so"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}
	// Remove a file that SHA256SUMS still lists.
	if err := os.Remove(filepath.Join(dir, "lib.so")); err != nil {
		t.Fatal(err)
	}
	if err := bundle.ValidateChecksums(dir); err == nil {
		t.Fatal("expected missing listed file to be rejected")
	}
}

func TestBundleExtractRejectsTooManyEntries(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "big.xqsp")

	zf, err := os.Create(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for i := 0; i < 5000; i++ {
		name := fmt.Sprintf("file%d.txt", i)
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	if err := bundle.Extract(bundlePath, dest); err == nil {
		t.Fatal("expected too many entries to fail")
	}
}
