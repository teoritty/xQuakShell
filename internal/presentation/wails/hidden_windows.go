//go:build windows

package wails

import (
	"path/filepath"
	"syscall"
)

func isHiddenLocal(fullPath, name string) bool {
	abs, err := filepath.Abs(fullPath)
	if err != nil {
		return name[0] == '.'
	}
	ptr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return name[0] == '.'
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return name[0] == '.'
	}
	const fileAttributeHidden = 0x02
	return (attrs & fileAttributeHidden) != 0
}
