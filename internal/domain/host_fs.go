package domain

import (
	"errors"
	"time"
)

// ErrHostPathInvalid is returned when a host filesystem path is malformed.
var ErrHostPathInvalid = errors.New("host path invalid")

// HostFileInfo describes basic metadata for a host filesystem path.
type HostFileInfo struct {
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// LocalFileEntry describes a file or directory entry for the local file browser.
type LocalFileEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
	Mode    string
	Owner   string
}

// HostAppLauncher opens files with the system default app or a specified editor.
type HostAppLauncher interface {
	OpenDefault(path string) error
	OpenWith(appPath, filePath string) error
}

// HostFileSystem provides trusted filesystem access for the core application UI.
//
// Security: This is an intentional design decision (ADR-007). The host process
// is the trust anchor — Local Files, SFTP transfers, and native file dialogs
// operate on the user's filesystem without a sandbox root. This differs from
// plugin FS (FSProxy), which is manifest-sandboxed, and from PortableDataStore,
// which is jailed to <exe>/data for application state only.
//
// Threat model: callers are Wails-bound host UI methods, not plugin IPC.
// Plugins cannot invoke this port; their only FS surface is fs.* RPC.
type HostFileSystem interface {
	DefaultPath() string
	ResolvePath(path string) (string, error)
	Stat(localPath string) (HostFileInfo, error)
	List(dirPath string, includeHidden bool, isHidden func(fullPath, name string) bool) ([]LocalFileEntry, error)
	Remove(localPath string) error
	Mkdir(dirPath string) error
	Rename(oldPath, newPath string) error
	CreateFile(localPath string) error
	// Copy copies srcPath (file or directory, recursively) into destDir,
	// keeping srcPath's base name. Symlinks are copied as links, not followed.
	Copy(srcPath, destDir string) error
	// CopyTo copies srcPath (file or directory, recursively) to the explicit
	// destPath, i.e. the caller chooses the destination's base name. This is what
	// lets a conflict "Rename" write under a fresh name. Symlinks are copied as
	// links, not followed.
	CopyTo(srcPath, destPath string) error
}
