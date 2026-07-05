//go:build windows

package host

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// OpenDefault opens path with the platform default application.
func (l *AppLauncher) OpenDefault(path string) error {
	return openWithShellExecute(path)
}

// OpenWith opens filePath with the application at appPath.
func (l *AppLauncher) OpenWith(appPath, filePath string) error {
	execPath, err := validateExternalEditor(appPath)
	if err != nil {
		return err
	}
	return exec.Command(execPath, filePath).Start()
}

func validateExternalEditor(editorPath string) (string, error) {
	if strings.ContainsAny(editorPath, "\r\n\x00") {
		return "", fmt.Errorf("invalid editor path")
	}
	abs, err := filepath.Abs(editorPath)
	if err != nil {
		return "", fmt.Errorf("invalid editor path")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("editor not found")
	}
	if info.IsDir() {
		return "", fmt.Errorf("editor path is a directory")
	}
	return abs, nil
}

var (
	modShell32        = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = modShell32.NewProc("ShellExecuteW")
)

const swShowDefault = 10

func openWithShellExecute(path string) error {
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
