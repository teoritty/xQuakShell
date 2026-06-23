//go:build windows

package wails

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32        = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = modShell32.NewProc("ShellExecuteW")
)

const swShowDefault = 10

func openWithSystemDefault(path string) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	verbPtr, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	ret, _, err := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
		swShowDefault,
	)
	if ret <= 32 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return fmt.Errorf("open file: %w", err)
		}
		return fmt.Errorf("open file failed")
	}
	return nil
}
