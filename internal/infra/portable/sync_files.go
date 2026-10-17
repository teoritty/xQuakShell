package portable

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/pathsafe"
)

// partialMarker names the temp files WriteFileAtomic writes beside their target. Walk leaves them
// out, so a write interrupted by a crash is never synced as a file of its own.
const partialMarker = ".xqs-partial-"

func denied(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrPortablePathDenied, err)
}

// resolveZonePath resolves path under the root and refuses a colon after the volume name: on NTFS
// "state.json:hidden" names an alternate data stream of state.json, a second file the walk can
// never see.
func (s *DataStore) resolveZonePath(path string) (string, error) {
	abs, err := s.ResolvePath(path)
	if err != nil {
		return "", err
	}
	if strings.Contains(abs[len(filepath.VolumeName(abs)):], ":") {
		return "", domain.ErrPortablePathDenied
	}
	return abs, nil
}

// Walk lists the regular files under dir. Symlinks are skipped, never followed, so a link planted
// in a plugin's directory cannot pull a file from elsewhere into sync.
func (s *DataStore) Walk(dir string, fn func(rel string, size int64) error) error {
	abs, err := s.resolveZonePath(dir)
	if err != nil {
		return err
	}
	// Every segment down to dir must be a plain directory: a junction halfway would make the walk
	// below list another tree's files under this plugin's names.
	if err := s.mkdirNoFollow(abs, false); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || strings.Contains(d.Name(), partialMarker) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		return fn(filepath.ToSlash(rel), fi.Size())
	})
}

// ReadRegularFile reads one regular file of at most maxBytes, through a descriptor opened without
// following links and re-checked against the root.
func (s *DataStore) ReadRegularFile(path string, maxBytes int64) ([]byte, error) {
	abs, err := s.resolveZonePath(path)
	if err != nil {
		return nil, err
	}
	if err := s.checkNoLinks(abs); err != nil {
		return nil, err
	}
	data, err := pathsafe.ReadExistingFile([]string{s.root}, abs, maxBytes)
	if errors.Is(err, pathsafe.ErrPathDenied) {
		return nil, denied(err)
	}
	return data, err
}

// WriteFileAtomic writes data to a temp file beside path and renames it over path.
func (s *DataStore) WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	if err := s.requireWritable(); err != nil {
		return err
	}
	abs, err := s.resolveZonePath(path)
	if err != nil {
		return err
	}
	if err := s.mkdirNoFollow(filepath.Dir(abs), true); err != nil {
		return err
	}
	if info, err := os.Lstat(abs); err == nil && !info.Mode().IsRegular() {
		return domain.ErrPortablePathDenied
	}
	tmp, err := s.writeTemp(abs, data, perm)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, abs); err != nil {
		if rmErr := os.Remove(tmp); rmErr != nil {
			return errors.Join(err, rmErr)
		}
		return err
	}
	return nil
}

func (s *DataStore) writeTemp(abs string, data []byte, perm fs.FileMode) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	tmp := filepath.Join(filepath.Dir(abs), "."+filepath.Base(abs)+partialMarker+hex.EncodeToString(suffix[:]))
	f, err := pathsafe.CreateNewFile([]string{s.root}, tmp, perm)
	if err != nil {
		return "", denied(err)
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err != nil {
		if rmErr := os.Remove(tmp); rmErr != nil {
			return "", errors.Join(err, rmErr)
		}
		return "", err
	}
	return tmp, nil
}

// mkdirNoFollow checks dir and its parents below the root one segment at a time, refusing a symlink,
// a junction or a file at any segment, and creates the missing ones when create is set. os.MkdirAll
// would walk through a symlinked directory and create the rest of the path wherever it points.
func (s *DataStore) mkdirNoFollow(dir string, create bool) error {
	rel, err := filepath.Rel(s.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return domain.ErrPortablePathDenied
	}
	current := s.root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == "." || seg == "" {
			continue
		}
		current = filepath.Join(current, seg)
		info, err := os.Lstat(current)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !create:
			return err
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
		case err != nil:
			return err
		case !info.IsDir():
			return domain.ErrPortablePathDenied
		}
	}
	return nil
}

// RemoveFile deletes one regular file, refusing links on the way exactly as a write does. Remove
// resolves its path lexically and so would delete through a junction planted halfway.
func (s *DataStore) RemoveFile(path string) error {
	if err := s.requireWritable(); err != nil {
		return err
	}
	abs, err := s.resolveZonePath(path)
	if err != nil {
		return err
	}
	if err := s.checkNoLinks(abs); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return os.Remove(abs)
}

// checkNoLinks requires every segment between the root and abs to be a plain directory and abs
// itself a regular file. A junction or mount point shows as neither, so it is refused too.
func (s *DataStore) checkNoLinks(abs string) error {
	if err := s.mkdirNoFollow(filepath.Dir(abs), false); err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return domain.ErrPortablePathDenied
	}
	return nil
}

var _ domain.SyncedFileZone = (*DataStore)(nil)
