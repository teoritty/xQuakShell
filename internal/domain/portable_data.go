package domain

import (
	"errors"
	"io/fs"
)

// ErrPortablePathDenied is returned when a path escapes the portable data root.
var ErrPortablePathDenied = errors.New("portable path access denied")

// PortableDataStore manages internal application state under the portable data root (ADR-006).
//
// All paths resolved through this port must stay under <exe>/data. Do not use this port
// for Local Files browser, SFTP transfers, or file dialogs — use HostFileSystem (ADR-007).
type PortableDataStore interface {
	DataRoot() string
	TempDir() string
	EnsureTempDir() (string, error)
	ResolvePath(path string) (string, error)
	Remove(path string) error
	ReadFile(path string) ([]byte, error)
}

// SyncedFileZone reads and writes the plugin data files that sync between devices, inside the same
// jailed root as PortableDataStore (ADR-007). Every path is resolved under the root, and a symlink,
// a directory or any other non-regular file in the way is refused rather than followed: the content
// written here comes from another device and is untrusted.
type SyncedFileZone interface {
	// Walk calls fn for every regular file under dir with its slash-separated path relative to dir.
	// Symlinks are neither listed nor followed; a dir that does not exist walks nothing.
	Walk(dir string, fn func(rel string, size int64) error) error
	// ReadRegularFile reads a regular file of at most maxBytes, refusing a symlink.
	ReadRegularFile(path string, maxBytes int64) ([]byte, error)
	// WriteFileAtomic replaces a file with data through a temp file and a rename in the same
	// directory, creating missing directories, so a reader never sees a partial file.
	WriteFileAtomic(path string, data []byte, perm fs.FileMode) error
	// Remove deletes a file or directory tree.
	Remove(path string) error
}
