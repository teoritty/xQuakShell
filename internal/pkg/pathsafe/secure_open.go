package pathsafe

import (
	"io"
	"os"
)

// OpenExistingFile opens an existing file under roots with TOCTOU re-validation on the descriptor.
func OpenExistingFile(roots []string, absPath string, write bool) (*os.File, error) {
	verified, err := SecurePathUnderRoots(absPath, roots)
	if err != nil {
		return nil, err
	}
	f, err := openExistingNoFollow(verified, write)
	if err != nil {
		return nil, err
	}
	if err := VerifyOpenFileUnderRoots(f, roots); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// OpenExistingDir opens an existing directory under roots with TOCTOU re-validation.
func OpenExistingDir(roots []string, absPath string) (*os.File, error) {
	verified, err := SecurePathUnderRoots(absPath, roots)
	if err != nil {
		return nil, err
	}
	f, err := openDirNoFollow(verified)
	if err != nil {
		return nil, err
	}
	if err := VerifyOpenFileUnderRoots(f, roots); err != nil {
		_ = f.Close()
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.IsDir() {
		_ = f.Close()
		return nil, ErrPathDenied
	}
	return f, nil
}

// CreateNewFile creates a new file under roots using exclusive create.
func CreateNewFile(roots []string, absPath string, perm os.FileMode) (*os.File, error) {
	verified, err := SecurePathUnderRoots(absPath, roots)
	if err != nil {
		return nil, err
	}
	f, err := createNewNoFollow(verified, perm)
	if err != nil {
		return nil, err
	}
	if err := VerifyOpenFileUnderRoots(f, roots); err != nil {
		_ = f.Close()
		_ = os.Remove(verified)
		return nil, err
	}
	return f, nil
}

// ReadExistingFile reads file contents with TOCTOU-safe open.
func ReadExistingFile(roots []string, absPath string, maxBytes int64) ([]byte, error) {
	f, err := OpenExistingFile(roots, absPath, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrPathDenied
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, ErrPathDenied
	}
	return io.ReadAll(io.LimitReader(f, maxBytes))
}

// WriteExistingFile replaces file contents with TOCTOU-safe truncate write.
func WriteExistingFile(roots []string, absPath string, data []byte, perm os.FileMode) error {
	f, err := OpenExistingFile(roots, absPath, true)
	if err != nil {
		if os.IsNotExist(err) {
			f, err = CreateNewFile(roots, absPath, perm)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	defer f.Close()

	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}
