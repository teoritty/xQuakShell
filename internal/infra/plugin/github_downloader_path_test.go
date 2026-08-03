package plugin

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeExtractPathAcceptsContainedNames(t *testing.T) {
	dest := t.TempDir()

	cases := []struct {
		name  string
		entry string
		want  string
	}{
		{"plain file", "tool.exe", "tool.exe"},
		{"nested file", "bin/tool", filepath.Join("bin", "tool")},
		{"backslash-free deep nesting", "a/b/c/tool", filepath.Join("a", "b", "c", "tool")},
		{"leading ./ is stripped", "./bin/tool", filepath.Join("bin", "tool")},
		{"interior .. that stays inside", "bin/sub/../tool", filepath.Join("bin", "tool")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safeExtractPath(dest, tc.entry)
			if err != nil {
				t.Fatalf("safeExtractPath(%q) = error %v, want success", tc.entry, err)
			}
			absDest, err := filepath.Abs(dest)
			if err != nil {
				t.Fatalf("abs dest: %v", err)
			}
			want := filepath.Join(filepath.Clean(absDest), tc.want)
			if got != want {
				t.Fatalf("safeExtractPath(%q) = %q, want %q", tc.entry, got, want)
			}
		})
	}
}

func TestSafeExtractPathRejectsEscapes(t *testing.T) {
	dest := t.TempDir()

	entries := []string{
		"../escape",
		"../../escape",
		"bin/../../escape",
		"..",
		"/etc/passwd",
		"/",
	}
	if runtime.GOOS == "windows" {
		entries = append(entries, `C:\Windows\System32\evil.dll`, `C:evil`, `\\server\share\evil`)
	}

	for _, entry := range entries {
		t.Run(entry, func(t *testing.T) {
			got, err := safeExtractPath(dest, entry)
			if err == nil {
				t.Fatalf("safeExtractPath(%q) = %q, want rejection", entry, got)
			}
			if errors.Is(err, errSkipEntry) {
				t.Fatalf("safeExtractPath(%q) was skipped, want rejection", entry)
			}
		})
	}
}

// `tar -czf x.tgz .` emits an entry for the archive root itself. It is contained,
// not an escape, and must not fail the extraction.
func TestSafeExtractPathSkipsArchiveRootEntry(t *testing.T) {
	dest := t.TempDir()
	for _, entry := range []string{".", "./", ""} {
		if _, err := safeExtractPath(dest, entry); !errors.Is(err, errSkipEntry) {
			t.Fatalf("safeExtractPath(%q) = %v, want errSkipEntry", entry, err)
		}
	}
}

func TestExtractZIPRejectsTraversalEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	d := NewBinaryDownloader(nil, dir)
	err = d.extractArchive(archive, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("extractArchive accepted a traversal entry")
	}
	if !strings.Contains(err.Error(), "invalid archive path") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "escaped.txt")); statErr == nil {
		t.Fatal("traversal entry was written outside the destination")
	}
}

func TestExtractTARRejectsTraversalEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.tar.gz")

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	body := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "../escaped.txt",
		Mode:     0o600,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	d := NewBinaryDownloader(nil, dir)
	err := d.extractArchive(archive, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("extractArchive accepted a traversal entry")
	}
	if !strings.Contains(err.Error(), "invalid archive path") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "escaped.txt")); statErr == nil {
		t.Fatal("traversal entry was written outside the destination")
	}
}

// A tarball produced by `tar -czf x.tgz .` carries a "./" root entry alongside
// real files; the whole archive must still extract.
func TestExtractTARAcceptsArchiveRootEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "ok.tar.gz")

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{Name: "./", Mode: 0o700, Typeflag: tar.TypeDir}); err != nil {
		t.Fatalf("write root header: %v", err)
	}
	body := []byte("#!/bin/sh\n")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "./bin/tool",
		Mode:     0o700,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	out := filepath.Join(dir, "out")
	d := NewBinaryDownloader(nil, dir)
	if err := d.extractArchive(archive, out); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "bin", "tool")); err != nil {
		t.Fatalf("expected extracted file: %v", err)
	}
}
