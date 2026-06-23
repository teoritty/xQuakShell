package domain

import "errors"

// ErrLocalPathDenied is returned when a path escapes the portable data root.
var ErrLocalPathDenied = errors.New("local path access denied")

// LocalFileEntry describes a file or directory entry for the local file browser.
type LocalFileEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
	Mode    string
}

// LocalFileSystem lists and mutates files under a portable data root.
type LocalFileSystem interface {
	DefaultPath() string
	Root() string
	TempDir() string
	EnsureTempDir() (string, error)
	ResolvePath(path string) (string, error)
	List(dirPath string, includeHidden bool, isHidden func(fullPath, name string) bool) ([]LocalFileEntry, error)
	Remove(localPath string) error
	Mkdir(dirPath string) error
	Rename(oldPath, newPath string) error
	CreateFile(localPath string) error
}
