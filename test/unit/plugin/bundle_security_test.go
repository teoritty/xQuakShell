package plugin_test

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/infra/plugin/bundle"
)

func TestBundleExtractRejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "evil.xqs-plugin")

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
	bundlePath := filepath.Join(dir, "dots.xqs-plugin")

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

func TestBundleExtractRejectsTooManyEntries(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "big.xqs-plugin")

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
