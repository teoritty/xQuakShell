package wails

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Local filesystem Wails handlers — routing map (single source of truth):
//
//	| Method                                                                 | Port         | Zone              |
//	|------------------------------------------------------------------------|--------------|-------------------|
//	| ListLocalPath, RemoveLocalPath, MkdirLocalPath, RenameLocalPath,     | hostFS       | Host user FS      |
//	| CreateLocalFile, OpenFileWithSystem, StartFileWatch,                   |              | (ADR-007)         |
//	| SelectLocalFile, SelectLocalDirectory                                  |              |                   |
//	| GetUserHomeDir                                                         | hostFS       | Host user FS      |
//	| GetPortableDataRoot                                                    | portableData | Portable app data |
//	| GetTempDir                                                             | portableData | Portable app data |

// ListLocalPath returns directory entries for a local path.
// includeHidden when false filters out hidden files (name starts with . on Unix, HIDDEN attribute on Windows).
func (a *AppAPI) ListLocalPath(dirPath string, includeHidden bool) ([]LocalNodeDTO, error) {
	if a.hostFS == nil {
		return nil, fmt.Errorf("local file service unavailable")
	}
	if dirPath == "" {
		dirPath = a.hostFS.DefaultPath()
	}
	nodes, err := a.hostFS.List(dirPath, includeHidden, isHiddenLocal)
	if err != nil {
		return nil, err
	}
	result := make([]LocalNodeDTO, 0, len(nodes))
	for _, node := range nodes {
		dto := LocalNodeDTO{
			Name:    node.Name,
			Path:    node.Path,
			IsDir:   node.IsDir,
			Size:    node.Size,
			ModTime: node.ModTime,
			Mode:    node.Mode,
		}
		if info, err := os.Stat(node.Path); err == nil {
			dto.Owner = getLocalFileOwner(info, node.Path)
		}
		result = append(result, dto)
	}
	return result, nil
}

// RemoveLocalPath deletes a local file or directory (recursively for directories).
func (a *AppAPI) RemoveLocalPath(localPath string) error {
	if a.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.Remove(localPath)
}

// MkdirLocalPath creates a local directory (and parents if needed).
func (a *AppAPI) MkdirLocalPath(dirPath string) error {
	if a.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.Mkdir(dirPath)
}

// RenameLocalPath renames a local file or directory.
func (a *AppAPI) RenameLocalPath(oldPath, newPath string) error {
	if a.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.Rename(oldPath, newPath)
}

// CreateLocalFile creates an empty local file.
func (a *AppAPI) CreateLocalFile(localPath string) error {
	if a.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.CreateFile(localPath)
}

// GetPortableDataRoot returns the portable data root (<exe>/data) for settings and plugin layout.
func (a *AppAPI) GetPortableDataRoot() (string, error) {
	if a.portableData == nil {
		return "", fmt.Errorf("portable data store unavailable")
	}
	return a.portableData.DataRoot(), nil
}

// GetUserHomeDir returns the user's home directory for the local file browser default path.
func (a *AppAPI) GetUserHomeDir() (string, error) {
	if a.hostFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.DefaultPath(), nil
}

// GetTempDir returns the portable temp directory under <exe>/data/tmp.
func (a *AppAPI) GetTempDir() (string, error) {
	if a.portableData == nil {
		return "", fmt.Errorf("portable data store unavailable")
	}
	return a.portableData.EnsureTempDir()
}

// StartFileWatch watches a local file for changes and emits FileEdited when mtime changes.
func (a *AppAPI) StartFileWatch(localPath string) {
	if a.hostFS == nil {
		return
	}
	abs, err := a.hostFS.ResolvePath(localPath)
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
func (a *AppAPI) OpenFileWithSystem(localPath, editorPath string) error {
	if a.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	abs, err := a.hostFS.ResolvePath(localPath)
	if err != nil {
		return err
	}
	if editorPath != "" {
		editorPath = strings.TrimSpace(editorPath)
		if editorPath != "" {
			execPath, err := validateExternalEditor(editorPath)
			if err != nil {
				return err
			}
			return exec.Command(execPath, abs).Start()
		}
	}
	switch runtime.GOOS {
	case "windows":
		return openWithSystemDefault(abs)
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
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

// --- File Dialogs ---

// SelectLocalFile opens a native file picker and returns the selected file path.
func (a *AppAPI) SelectLocalFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	path, err := wailsrt.OpenFileDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select File",
	})
	if err != nil || path == "" {
		return path, err
	}
	return a.resolveHostLocalPath(path)
}

// SelectLocalDirectory opens a native directory picker.
func (a *AppAPI) SelectLocalDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	path, err := wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select Directory",
	})
	if err != nil || path == "" {
		return path, err
	}
	return a.resolveHostLocalPath(path)
}

func (a *AppAPI) resolveHostLocalPath(path string) (string, error) {
	if a.hostFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return a.hostFS.ResolvePath(path)
}
