package sftp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// downloadTempAttempts bounds the search for an unused temp name. With 64 bits of randomness a
	// second attempt is already implausible; the loop exists so a directory that refuses every
	// name - a full disk reported as EEXIST by some filesystems, a hostile local process racing us -
	// ends in an error instead of spinning.
	downloadTempAttempts = 8

	// downloadTempBaseLimit keeps the temp name inside the 255-byte limit most filesystems put on a
	// single component, since the decoration adds fifteen bytes to a name the remote side chose.
	downloadTempBaseLimit = 200
)

var errTempNameExhausted = errors.New("no unused temporary name available")

// downloadTarget is the local side of a download: a sibling temp file that becomes the real file
// only once the last byte is written.
//
// Writing straight to the destination made two failures destructive rather than merely
// unsuccessful. os.Create truncates, so the existing file was gone before the first byte arrived —
// a download that failed halfway left the user with neither the old file nor the new one. And what
// remained wore the production name, with nothing to mark it incomplete: a truncated archive, a
// half-written config, a binary missing its tail. A dropped connection costs a hostile server
// nothing to arrange, and the user has no way to tell the result from a complete file.
//
// The temp file is a sibling rather than a file in TMPDIR because a rename is only atomic — and
// only cheap — within one filesystem, and the destination directory is the one filesystem we know
// the file fits on.
type downloadTarget struct {
	file      *os.File
	tempPath  string
	finalPath string
	committed bool
}

// newDownloadTarget opens the temp file that will become finalPath.
func newDownloadTarget(finalPath string) (*downloadTarget, error) {
	dir := filepath.Dir(finalPath)
	base := filepath.Base(finalPath)
	if len(base) > downloadTempBaseLimit {
		base = base[:downloadTempBaseLimit]
	}
	var lastErr error
	for range downloadTempAttempts {
		suffix, err := randomSuffix()
		if err != nil {
			return nil, err
		}
		tempPath := filepath.Join(dir, fmt.Sprintf(".%s.%s.part", base, suffix))
		// O_EXCL is what makes an unguessed name safe: a collision is an error rather than a second
		// writer, so neither a concurrent download of the same file nor a local process that
		// pre-created the name can end up sharing this file handle. It also refuses a symlink
		// planted at the name, which would otherwise redirect the write.
		// 0600 rather than the 0666 os.Create used. The old mode let the umask decide, which on a
		// stock system is 0644 - every local account could read a file just pulled off a remote
		// host, and nothing in a download implies the user wanted it shared with them. The mode is
		// applied to the temp file and survives the rename, so the finished download carries it.
		f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return &downloadTarget{file: f, tempPath: tempPath, finalPath: finalPath}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%w for %s: %w", errTempNameExhausted, finalPath, lastErr)
}

// Commit closes the temp file and moves it onto the destination.
//
// The Close error is returned rather than discarded: on a network filesystem a write failure can
// surface only here, and reporting success on a file the kernel could not finish writing is the
// same lie the temp file exists to prevent.
func (t *downloadTarget) Commit() error {
	if err := t.file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", t.tempPath, err)
	}
	if err := os.Rename(t.tempPath, t.finalPath); err != nil {
		return fmt.Errorf("rename %s into place: %w", t.tempPath, err)
	}
	t.committed = true
	return nil
}

// Discard removes the incomplete file. It is safe to defer unconditionally: after a successful
// Commit there is nothing at tempPath, and the flag keeps this from touching the file that is now
// the user's download.
func (t *downloadTarget) Discard() {
	if t.committed {
		return
	}
	_ = t.file.Close()
	_ = os.Remove(t.tempPath)
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("temporary file name: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
