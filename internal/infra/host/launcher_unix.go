//go:build !windows

package host

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenDefault opens path with the platform default application.
func (l *AppLauncher) OpenDefault(path string) error {
	switch runtime.GOOS {
	case "darwin":
		// #nosec G204 -- fixed command name; path is an argument, and exec.Command
		// does not go through a shell, so it cannot expand into another command.
		return exec.Command("open", path).Start()
	default:
		// #nosec G204 -- fixed command name; see above.
		return exec.Command("xdg-open", path).Start()
	}
}

// OpenWith opens filePath with the application at appPath.
func (l *AppLauncher) OpenWith(appPath, filePath string) error {
	execPath, err := validateExternalEditor(appPath)
	if err != nil {
		return err
	}
	// #nosec G204 -- opening a file in a user-chosen editor is the feature. execPath
	// has passed validateExternalEditor, and filePath is an argument, not a shell word.
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
