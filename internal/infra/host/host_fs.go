package host

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/pathsafe"
)

// HostFS implements domain.HostFileSystem for trusted host UI access (ADR-007).
//
// Security: see domain.HostFileSystem. This adapter intentionally does not sandbox
// paths to <exe>/data. Plugin sandboxing remains in capability.FSProxy.
type HostFS struct {
	defaultPath string
}

// NewHostFS creates a host filesystem adapter rooted at the user's home directory.
func NewHostFS() *HostFS {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		abs = filepath.Clean(home)
	} else {
		abs = filepath.Clean(abs)
	}
	return &HostFS{defaultPath: abs}
}

// DefaultPath returns the user's home directory.
func (fs *HostFS) DefaultPath() string {
	return fs.defaultPath
}

// ResolvePath normalizes path without a sandbox root.
func (fs *HostFS) ResolvePath(path string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("host file service unavailable")
	}
	if path == "" {
		return fs.defaultPath, nil
	}
	resolved, err := pathsafe.ResolveHostPath(path)
	if err != nil {
		return "", domain.ErrHostPathInvalid
	}
	return resolved, nil
}

// Stat returns basic metadata for a host filesystem path.
func (fs *HostFS) Stat(localPath string) (domain.HostFileInfo, error) {
	path, err := fs.ResolvePath(localPath)
	if err != nil {
		return domain.HostFileInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return domain.HostFileInfo{}, err
	}
	return domain.HostFileInfo{
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

// List returns directory entries under path.
func (fs *HostFS) List(dirPath string, includeHidden bool, isHidden func(fullPath, name string) bool) ([]domain.LocalFileEntry, error) {
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
			Owner:   fileOwner(info),
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
func (fs *HostFS) Remove(localPath string) error {
	path, err := fs.ResolvePath(localPath)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Mkdir creates a directory and parents.
func (fs *HostFS) Mkdir(dirPath string) error {
	path, err := fs.ResolvePath(dirPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

// Rename moves a file or directory on the host filesystem.
func (fs *HostFS) Rename(oldPath, newPath string) error {
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

// CreateFile creates an empty file on the host filesystem.
func (fs *HostFS) CreateFile(localPath string) error {
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

// Copy copies srcPath (file or directory tree) into destDir, keeping srcPath's
// base name. Used for copying files dropped from the OS file explorer into
// the local file browser. Symlinks are recreated as links, not followed —
// this avoids infinite recursion on symlinked directories.
func (fs *HostFS) Copy(srcPath, destDir string) error {
	src, err := fs.ResolvePath(srcPath)
	if err != nil {
		return err
	}
	dest, err := fs.ResolvePath(destDir)
	if err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	target := filepath.Join(dest, filepath.Base(src))
	if info.IsDir() {
		return copyDirRecursive(src, target)
	}
	return copyEntry(src, target, info)
}

func copyDirRecursive(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(srcDir, e.Name())
		destPath := filepath.Join(destDir, e.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		if err := copyEntry(srcPath, destPath, info); err != nil {
			return err
		}
	}
	return nil
}

// copyEntry copies a single file or symlink. Symlinks are recreated pointing
// at the same target rather than having their contents followed and copied.
func copyEntry(srcPath, destPath string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(srcPath)
		if err != nil {
			return err
		}
		_ = os.Remove(destPath)
		return os.Symlink(linkTarget, destPath)
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

var _ domain.HostFileSystem = (*HostFS)(nil)
