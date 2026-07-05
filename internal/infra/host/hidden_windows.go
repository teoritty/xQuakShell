//go:build windows

package host

import (
	"path/filepath"
	"syscall"
)

// IsHiddenLocal reports whether a local file entry should be treated as hidden.
func IsHiddenLocal(fullPath, name string) bool {
	abs, err := filepath.Abs(fullPath)
	if err != nil {
		return len(name) > 0 && name[0] == '.'
	}
	ptr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return len(name) > 0 && name[0] == '.'
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return len(name) > 0 && name[0] == '.'
	}
	const fileAttributeHidden = 0x02
	return (attrs & fileAttributeHidden) != 0
}
