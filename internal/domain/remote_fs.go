package domain

import (
	"context"
	"time"
)

// RemoteNode represents a single entry (file or directory) in the remote filesystem.
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

	// RemoveAll recursively deletes a remote path (file or directory with contents).
	RemoveAll(ctx context.Context, path string) error

	// Rename moves/renames a remote path.
	Rename(ctx context.Context, oldPath, newPath string) error

	// Close releases the underlying SFTP connection.
	Close() error
}
