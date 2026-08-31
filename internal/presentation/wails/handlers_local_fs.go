package wails

import (
	"fmt"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
)

// Local filesystem Wails handlers — routing map (single source of truth):
//
//	| Method                                                                 | Port         | Zone              |
//	|------------------------------------------------------------------------|--------------|-------------------|
//	| ListLocalPath, RemoveLocalPath, MkdirLocalPath, RenameLocalPath,     | localFS      | Host user FS      |
//	| CreateLocalFile, CopyLocalPath, OpenFileWithSystem, StartFileWatch,    |              | (ADR-007)         |
//	| SelectLocalFile, SelectLocalDirectory                                  |              |                   |
//	| GetUserHomeDir                                                         | localFS      | Host user FS      |
//	| GetPortableDataRoot                                                    | portableData | Portable app data |
//	| GetTempDir                                                             | portableData | Portable app data |

// ListLocalPath returns directory entries for a local path.
// includeHidden when false filters out hidden files (name starts with . on Unix, HIDDEN attribute on Windows).
func (a *AppAPI) ListLocalPath(dirPath string, includeHidden bool) ([]LocalNodeDTO, error) {
	if a.localFS == nil {
		return nil, fmt.Errorf("local file service unavailable")
	}
	nodes, err := a.localFS.List(dirPath, includeHidden)
	if err != nil {
		return nil, err
	}
	result := make([]LocalNodeDTO, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, LocalNodeDTO{
			Name:    node.Name,
			Path:    node.Path,
			IsDir:   node.IsDir,
			Size:    node.Size,
			ModTime: node.ModTime,
			Mode:    node.Mode,
			Owner:   node.Owner,
		})
	}
	return result, nil
}

// RemoveLocalPath deletes a local file or directory (recursively for directories).
func (a *AppAPI) RemoveLocalPath(localPath string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.Remove(localPath)
}

// MkdirLocalPath creates a local directory (and parents if needed).
func (a *AppAPI) MkdirLocalPath(dirPath string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.Mkdir(dirPath)
}

// RenameLocalPath renames a local file or directory.
func (a *AppAPI) RenameLocalPath(oldPath, newPath string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.Rename(oldPath, newPath)
}

// CreateLocalFile makes an empty file so the browser can offer "new file"
// without shipping an editor. The path is validated by the service, not here:
// the frontend supplied it and is not a trust boundary.
func (a *AppAPI) CreateLocalFile(localPath string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.CreateFile(localPath)
}

// CopyLocalPath copies a local file or directory (recursively) into destDir,
// keeping its base name. Used for OS drag-and-drop into the local file browser.
func (a *AppAPI) CopyLocalPath(srcPath, destDir string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.Copy(srcPath, destDir)
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
	if a.localFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return a.localFS.DefaultPath()
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
	if a.localFS == nil {
		return
	}
	a.localFS.StartFileWatch(localPath, func() {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventFileEdited, map[string]string{"localPath": localPath})
		}
	})
}

// OpenFileWithSystem opens a local file with the system's default application or the specified editor.
func (a *AppAPI) OpenFileWithSystem(localPath, editorPath string) error {
	if a.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return a.localFS.OpenWithSystem(localPath, editorPath)
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

// SaveRecoveryKeyFile asks the user where to put the recovery key and writes it there, reporting
// whether a file was actually written.
//
// It takes no argument. The key it writes is the one the backend is holding for the dialog that is
// currently open, which is what keeps anything running in the WebView from asking to have an
// arbitrary string saved to an arbitrary path with a native picker in front of it. Once the user
// presses Done the held key is dropped and this refuses.
//
// A false return with a nil error is the user cancelling the dialog, which is not a failure and
// must not be reported as one - the key is still on screen and still theirs to copy.
//
// wailsrt.SaveFileDialog is the same call on Windows, Linux and macOS; the native dialog it opens is
// the platform's own, which is also what asks about overwriting an existing file.
func (a *AppAPI) SaveRecoveryKeyFile() (bool, error) {
	// Asked before anything else, including whether there is a window to put a dialog on. Whether a
	// key is held is the authorisation question, and answering it first is what guarantees no caller
	// can make a native save picker appear without one.
	key, ok := a.pendingRecovery.peek()
	if !ok {
		return false, domain.ErrRecoveryKeyUnavailable
	}
	if a.ctx == nil {
		return false, fmt.Errorf("no wails context")
	}

	path, err := wailsrt.SaveFileDialog(a.ctx, wailsrt.SaveDialogOptions{
		Title:           "Save recovery key",
		DefaultFilename: "xquakshell-recovery-key.txt",
		Filters: []wailsrt.FileFilter{
			{DisplayName: "Text file (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil || path == "" {
		return false, err
	}

	resolved, err := a.resolveHostLocalPath(path)
	if err != nil {
		return false, err
	}
	// The key alone, one line, with the trailing newline every text editor expects. Nothing names
	// the application or the vault: a file found on a shared machine should not announce what it
	// opens.
	if err := a.localFS.WriteSecretFile(resolved, []byte(domain.FormatRecoveryKey(key)+"\n")); err != nil {
		return false, err
	}
	return true, nil
}

func (a *AppAPI) resolveHostLocalPath(path string) (string, error) {
	if a.localFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return a.localFS.ResolvePath(path)
}
