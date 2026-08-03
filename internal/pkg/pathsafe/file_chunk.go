package pathsafe

import (
	"io"
	"os"
)

// FileChunk is a byte range read from a sandboxed file.
type FileChunk struct {
	Data      []byte
	Offset    int64
	TotalSize int64
	EOF       bool
}

// ReadFileChunk reads up to maxBytes from absPath starting at offset.
func ReadFileChunk(roots []string, absPath string, offset, maxBytes int64) (FileChunk, error) {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	f, err := OpenExistingFile(roots, absPath, false)
	if err != nil {
		return FileChunk{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return FileChunk{}, err
	}
	if info.IsDir() {
		return FileChunk{}, ErrPathDenied
	}
	total := info.Size()
	if offset < 0 || offset > total {
		return FileChunk{}, ErrPathDenied
	}
	if total-offset < maxBytes {
		maxBytes = total - offset
	}
	buf := make([]byte, maxBytes)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return FileChunk{}, err
	}
	return FileChunk{
		Data:      buf[:n],
		Offset:    offset,
		TotalSize: total,
		EOF:       offset+int64(n) >= total,
	}, nil
}

// WriteFileChunk writes data at offset, creating the file when missing.
func WriteFileChunk(roots []string, absPath string, offset int64, data []byte, perm os.FileMode) error {
	if offset < 0 {
		return ErrPathDenied
	}
	var (
		f   *os.File
		err error
	)
	if offset == 0 {
		f, err = OpenExistingFile(roots, absPath, true)
		if err != nil {
			if os.IsNotExist(err) {
				f, err = CreateNewFile(roots, absPath, perm)
			} else {
				return err
			}
		}
		if err == nil {
			if err := f.Truncate(0); err != nil {
				_ = f.Close()
				return err
			}
		}
	} else {
		f, err = OpenExistingFile(roots, absPath, true)
		if err != nil {
			if os.IsNotExist(err) {
				f, err = CreateNewFile(roots, absPath, perm)
			} else {
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	defer f.Close()

	end := offset + int64(len(data))
	if end < 0 {
		return ErrPathDenied
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if end > info.Size() {
		if err := f.Truncate(end); err != nil {
			return err
		}
	}
	if _, err := f.WriteAt(data, offset); err != nil {
		return err
	}
	return nil
}
