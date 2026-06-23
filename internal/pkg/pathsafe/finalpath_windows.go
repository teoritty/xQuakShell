//go:build windows

package pathsafe

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetFinalPathNameByHandle = modKernel32.NewProc("GetFinalPathNameByHandleW")
)

const finalPathBufferSize = 512

// FinalPath returns the absolute path of an open file descriptor.
func FinalPath(f *os.File) (string, error) {
	if f == nil {
		return "", fmt.Errorf("nil file")
	}
	handle := windows.Handle(f.Fd())
	buf := make([]uint16, finalPathBufferSize)
	n, _, err := procGetFinalPathNameByHandle.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if n == 0 {
		return "", fmt.Errorf("GetFinalPathNameByHandle: %w", err)
	}
	path := windows.UTF16ToString(buf[:n])
	if len(path) >= 4 && path[:4] == `\\?\` {
		path = path[4:]
	}
	return filepath.Clean(path), nil
}
