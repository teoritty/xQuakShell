package wails

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// sanitizeLocalPath normalizes a local path to prevent basic traversal attacks.
func sanitizeLocalPath(p string) string {
	return filepath.Clean(p)
}

// ListLocalPath returns directory entries for a local path.
// includeHidden when false filters out hidden files (name starts with . on Unix, HIDDEN attribute on Windows).
func (a *AppAPI) ListLocalPath(dirPath string, includeHidden bool) ([]LocalNodeDTO, error) {
	if dirPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dirPath = home
	}
	dirPath = sanitizeLocalPath(dirPath)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var result []LocalNodeDTO
	for _, e := range entries {
		if !includeHidden && isHiddenLocal(filepath.Join(dirPath, e.Name()), e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dirPath, e.Name())
		dto := LocalNodeDTO{
			Name:    e.Name(),
			Path:    fullPath,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Mode:    info.Mode().String(),
		}
		dto.Owner = getLocalFileOwner(info, fullPath)
		result = append(result, dto)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// RemoveLocalPath deletes a local file or directory (recursively for directories).
func (a *AppAPI) RemoveLocalPath(localPath string) error {
	return os.RemoveAll(sanitizeLocalPath(localPath))
}

// MkdirLocalPath creates a local directory (and parents if needed).
func (a *AppAPI) MkdirLocalPath(dirPath string) error {
	return os.MkdirAll(sanitizeLocalPath(dirPath), 0o755)
}

// RenameLocalPath renames a local file or directory.
func (a *AppAPI) RenameLocalPath(oldPath, newPath string) error {
	return os.Rename(sanitizeLocalPath(oldPath), sanitizeLocalPath(newPath))
}

// CreateLocalFile creates an empty local file.
func (a *AppAPI) CreateLocalFile(localPath string) error {
	f, err := os.Create(sanitizeLocalPath(localPath))
	if err != nil {
		return err
	}
	return f.Close()
}

// GetUserHomeDir returns the current user's home directory.
func (a *AppAPI) GetUserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// GetTempDir returns the system temp directory.
func (a *AppAPI) GetTempDir() (string, error) {
	return os.TempDir(), nil
}

// StartFileWatch watches a local file for changes and emits FileEdited when mtime changes.
// Polls every 500ms; stops after first change or after 1 hour.
func (a *AppAPI) StartFileWatch(localPath string) {
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	initialMod := info.ModTime()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(time.Hour)
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				info, err := os.Stat(abs)
				if err != nil {
					return
				}
				if info.ModTime().After(initialMod) {
					if a.ctx != nil {
						wailsrt.EventsEmit(a.ctx, EventFileEdited, map[string]string{"localPath": localPath})
					}
					return
				}
			}
		}
	}()
}

// OpenFileWithSystem opens a local file with the system's default application or the specified editor.
// If editorPath is non-empty, runs editorPath with localPath as argument; otherwise uses system default.
func (a *AppAPI) OpenFileWithSystem(localPath, editorPath string) error {
	abs, err := filepath.Abs(sanitizeLocalPath(localPath))
	if err != nil {
		return err
	}
	if editorPath != "" {
		editorPath = strings.TrimSpace(editorPath)
		if editorPath != "" {
			return exec.Command(editorPath, abs).Start()
		}
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/C", "start", "", abs).Start()
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}

// --- File Dialogs ---

// SelectLocalFile opens a native file picker and returns the selected file path.
func (a *AppAPI) SelectLocalFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenFileDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select File",
	})
}

// SelectLocalDirectory opens a native directory picker.
func (a *AppAPI) SelectLocalDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select Directory",
	})
}
