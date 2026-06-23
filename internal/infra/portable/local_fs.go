package portable

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/pathsafe"
)

// LocalFS lists and mutates files under a portable data root.
type LocalFS struct {
	root     string
	tempDir  string
	portable domain.PortableRuntime
}

// NewLocalFS creates a local filesystem adapter rooted at dataRoot.
func NewLocalFS(dataRoot, tempDir string, portableRuntime domain.PortableRuntime) *LocalFS {
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		abs = filepath.Clean(dataRoot)
	} else {
		abs = filepath.Clean(abs)
	}
	tempAbs := tempDir
	if tempDir != "" {
		if resolved, err := filepath.Abs(tempDir); err == nil {
			tempAbs = filepath.Clean(resolved)
		}
	}
	return &LocalFS{
		root:     abs,
		tempDir:  tempAbs,
		portable: portableRuntime,
	}
}

// DefaultPath returns the service root when no path is provided.
func (fs *LocalFS) DefaultPath() string {
	return fs.root
}

// Root returns the absolute portable data root.
func (fs *LocalFS) Root() string {
	return fs.root
}

// TempDir returns the portable temp directory path.
func (fs *LocalFS) TempDir() string {
	return fs.tempDir
}

// EnsureTempDir creates the portable temp directory when writable.
func (fs *LocalFS) EnsureTempDir() (string, error) {
	if fs == nil || fs.tempDir == "" {
		return "", fmt.Errorf("local temp directory unavailable")
	}
	if err := fs.requireWritable(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(fs.tempDir, 0o700); err != nil {
		return "", err
	}
	return fs.tempDir, nil
}

// ResolvePath normalizes path and verifies it stays under the data root.
func (fs *LocalFS) ResolvePath(path string) (string, error) {
	if fs == nil || fs.root == "" {
		return "", fmt.Errorf("local file service unavailable")
	}
	if path == "" {
		return fs.root, nil
	}
	resolved, err := pathsafe.ResolveUnderRoot(fs.root, path)
	if err != nil {
		return "", domain.ErrLocalPathDenied
	}
	return resolved, nil
}

// List returns directory entries under path.
func (fs *LocalFS) List(dirPath string, includeHidden bool, isHidden func(fullPath, name string) bool) ([]domain.LocalFileEntry, error) {
	dirPath, err := fs.ResolvePath(dirPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LocalFileEntry, 0, len(entries))
	for _, e := range entries {
		fullPath := filepath.Join(dirPath, e.Name())
		if !includeHidden && isHidden != nil && isHidden(fullPath, e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, domain.LocalFileEntry{
			Name:    e.Name(),
			Path:    fullPath,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Mode:    info.Mode().String(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Remove deletes a file or directory tree.
func (fs *LocalFS) Remove(localPath string) error {
	if err := fs.requireWritable(); err != nil {
		return err
	}
	path, err := fs.ResolvePath(localPath)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Mkdir creates a directory and parents.
func (fs *LocalFS) Mkdir(dirPath string) error {
	if err := fs.requireWritable(); err != nil {
		return err
	}
	path, err := fs.ResolvePath(dirPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

// Rename moves a file or directory within the data root.
func (fs *LocalFS) Rename(oldPath, newPath string) error {
	if err := fs.requireWritable(); err != nil {
		return err
	}
	oldAbs, err := fs.ResolvePath(oldPath)
	if err != nil {
		return err
	}
	newAbs, err := fs.ResolvePath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

// CreateFile creates an empty file under the data root.
func (fs *LocalFS) CreateFile(localPath string) error {
	if err := fs.requireWritable(); err != nil {
		return err
	}
	path, err := fs.ResolvePath(localPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func (fs *LocalFS) requireWritable() error {
	if fs == nil || fs.portable == nil {
		return nil
	}
	return fs.portable.RequireWritable()
}

var _ domain.LocalFileSystem = (*LocalFS)(nil)
