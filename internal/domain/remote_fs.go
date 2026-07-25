package domain

import (
	"context"
	"os"
	"time"
)

// ApplyTarget filters which descendants a recursive chmod/chown applies to.
// It never affects the root path itself, which is always changed.
type ApplyTarget int

const (
	ApplyBoth ApplyTarget = iota
	ApplyFilesOnly
	ApplyDirsOnly
)

// Matches reports whether an entry with the given isDir should be changed.
func (a ApplyTarget) Matches(isDir bool) bool {
	switch a {
	case ApplyFilesOnly:
		return !isDir
	case ApplyDirsOnly:
		return isDir
	default:
		return true
	}
}

// RemoteNode represents a single entry (file or directory) in the remote filesystem.
//
// Name is a single path segment, already validated by the adapter: it never
// contains a separator, never equals "." or "..", and is safe to join into a
// local path. Adapters MUST enforce this — consumers rely on it.
type RemoteNode struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Mode    string    `json:"mode,omitempty"`   // e.g. "rwxr-xr-x"
	Owner   string    `json:"owner,omitempty"`   // owner name or UID
	Group   string    `json:"group,omitempty"`   // group name or GID
}

// ProgressFunc is called during file transfers to report progress.
// done is the number of bytes transferred so far; total is the full size (-1 if unknown).
type ProgressFunc func(done, total int64)

// RemoteFS defines operations on a remote filesystem (SFTP).
type RemoteFS interface {
	// GetWorkingDirectory returns the remote server's current working directory (typically user's home).
	GetWorkingDirectory(ctx context.Context) (string, error)

	// List returns the direct children of the given remote directory.
	List(ctx context.Context, path string) ([]RemoteNode, error)

	// Upload copies a local file to the remote path, calling progress periodically.
	Upload(ctx context.Context, localPath, remotePath string, progress ProgressFunc) error

	// Download copies a remote file to the local path, calling progress periodically.
	Download(ctx context.Context, remotePath, localPath string, progress ProgressFunc) error

	// UploadRecursive recursively uploads a local directory to the remote path.
	UploadRecursive(ctx context.Context, localDir, remoteDir string, progress ProgressFunc) error

	// DownloadRecursive recursively downloads a remote directory to the local path.
	DownloadRecursive(ctx context.Context, remoteDir, localDir string, progress ProgressFunc) error

	// Mkdir creates a remote directory (and parents if needed).
	Mkdir(ctx context.Context, path string) error

	// CreateFile creates an empty remote file.
	CreateFile(ctx context.Context, path string) error

	// Remove deletes a remote file or empty directory.
	Remove(ctx context.Context, path string) error

	// RemoveAll recursively deletes a remote path (file or directory with
	// contents). onEach, if non-nil, is invoked once per removed entry so callers
	// can report progress; it must not block.
	RemoveAll(ctx context.Context, path string, onEach func()) error

	// CountTree returns how many entries a recursive operation with the given
	// applyTo filter would act on (root plus matching descendants). It is
	// read-only and is used to pre-compute a progress total. onEach, if non-nil,
	// is invoked once per counted entry so callers can show a live scan counter;
	// it must not block.
	CountTree(ctx context.Context, path string, applyTo ApplyTarget, onEach func()) (int64, error)

	// Rename moves/renames a remote path.
	Rename(ctx context.Context, oldPath, newPath string) error

	// Chmod sets permission bits on a remote path.
	Chmod(ctx context.Context, path string, mode os.FileMode) error

	// Chown sets the owner uid/gid on a remote path.
	Chown(ctx context.Context, path string, uid, gid int) error

	// ChmodRecursive applies mode to path and, if it's a directory, to its
	// descendants filtered by applyTo. onEach, if non-nil, is invoked once per
	// changed entry for progress reporting; it must not block.
	ChmodRecursive(ctx context.Context, path string, mode os.FileMode, applyTo ApplyTarget, onEach func()) error

	// ChownRecursive applies uid/gid to path and, if it's a directory, to its
	// descendants filtered by applyTo. onEach, if non-nil, is invoked once per
	// changed entry for progress reporting; it must not block.
	ChownRecursive(ctx context.Context, path string, uid, gid int, applyTo ApplyTarget, onEach func()) error

	// Close releases the underlying SFTP connection.
	Close() error
}
