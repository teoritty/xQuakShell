package sftp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func partFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var parts []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			parts = append(parts, e.Name())
		}
	}
	return parts
}

// The download used to open the destination with os.Create, which truncates. The existing file was
// therefore destroyed before the first byte of the replacement arrived, so a transfer that failed
// halfway left the user with neither copy — and a dropped connection costs a hostile server nothing
// to arrange.
func TestAFailedDownloadLeavesThePreviousFileIntact(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(final, []byte("the original"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget: %v", err)
	}
	if _, err := target.file.WriteString("half of the rep"); err != nil {
		t.Fatalf("partial write: %v", err)
	}
	target.Discard()

	if got := readFile(t, final); got != "the original" {
		t.Errorf("after a failed download the file reads %q, want the untouched original", got)
	}
	if parts := partFiles(t, dir); len(parts) != 0 {
		t.Errorf("the incomplete file was left behind as %v", parts)
	}
}

// Nothing may wear the destination name until it is complete: a truncated archive or a binary
// missing its tail is indistinguishable from a whole one once it carries the real name.
func TestTheDestinationDoesNotExistUntilCommit(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "fresh.bin")

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget: %v", err)
	}
	defer target.Discard()
	if _, err := target.file.WriteString("partial"); err != nil {
		t.Fatalf("partial write: %v", err)
	}

	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("stat %s = %v, want the destination not to exist mid-transfer", final, err)
	}
	if parts := partFiles(t, dir); len(parts) != 1 {
		t.Fatalf("found %v, want exactly one in-progress file", parts)
	}
}

func TestCommitReplacesTheDestinationAndClearsTheTempFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(final, []byte("the original"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget: %v", err)
	}
	if _, err := target.file.WriteString("the replacement"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := target.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got := readFile(t, final); got != "the replacement" {
		t.Errorf("after commit the file reads %q, want the downloaded content", got)
	}
	if parts := partFiles(t, dir); len(parts) != 0 {
		t.Errorf("commit left %v behind", parts)
	}
}

// Discard is deferred unconditionally, so it runs after every successful download too — and by then
// the name it was going to delete belongs to nobody, or to somebody else. The temp names are drawn
// at random from one pool, so once this download has renamed its file away the next download may
// legitimately claim the name this one is still holding a path to. Discard must therefore be inert
// after a commit rather than merely harmless, or a finished download can delete a running one.
func TestDiscardAfterCommitTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "kept.txt")

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget: %v", err)
	}
	if _, err := target.file.WriteString("kept"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := target.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Stand in for the next download drawing the name this one has finished with.
	if err := os.WriteFile(target.tempPath, []byte("another download"), 0o600); err != nil {
		t.Fatalf("seed the reused name: %v", err)
	}
	target.Discard()

	if got := readFile(t, final); got != "kept" {
		t.Errorf("file reads %q after Discard ran on a committed target, want it untouched", got)
	}
	if _, err := os.Stat(target.tempPath); err != nil {
		t.Errorf("Discard deleted the file at its old temp name: %v", err)
	}
}

// Two downloads of the same file must not end up writing through one another's handle, so the temp
// name is per-attempt and claimed with O_EXCL.
func TestConcurrentDownloadsOfTheSameFileGetSeparateTempFiles(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "same.bin")

	first, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	defer first.Discard()
	second, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	defer second.Discard()

	if first.tempPath == second.tempPath {
		t.Fatalf("both downloads claimed %s", first.tempPath)
	}
	if parts := partFiles(t, dir); len(parts) != 2 {
		t.Errorf("found %v, want two independent in-progress files", parts)
	}
}

// A downloaded file is remote data landing in a local directory, and nothing about asking for it
// implies wanting every other account on the machine to read it. os.Create left that to the umask,
// which on a stock system means 0644.
func TestADownloadedFileIsNotReadableByOtherLocalUsers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file modes carry only the read-only bit; the permission has no meaning here")
	}
	dir := t.TempDir()
	final := filepath.Join(dir, "secret.env")

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget: %v", err)
	}
	if _, err := target.file.WriteString("TOKEN=..."); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := target.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	info, err := os.Stat(final)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("downloaded file mode is %#o; group and other must have no access", perm)
	}
}

// A name the remote side chose can be at the filesystem's length limit already; decorating it must
// not push the temp name past what the directory will accept.
func TestALongRemoteNameStillGetsATempFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, strings.Repeat("n", 240)+".bin")

	target, err := newDownloadTarget(final)
	if err != nil {
		t.Fatalf("newDownloadTarget on a 244-byte name: %v", err)
	}
	defer target.Discard()

	if got := len(filepath.Base(target.tempPath)); got > 255 {
		t.Errorf("temp name is %d bytes, past the 255-byte component limit", got)
	}
}
