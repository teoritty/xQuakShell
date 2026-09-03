package usecase

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

const (
	localFSWatchInterval = 500 * time.Millisecond
	localFSWatchTimeout  = time.Hour
)

type LocalFSService struct {
	hostFS   domain.HostFileSystem
	launcher domain.HostAppLauncher
	isHidden func(fullPath, name string) bool
}

type LocalFSServiceConfig struct {
	HostFS   domain.HostFileSystem
	Launcher domain.HostAppLauncher
	IsHidden func(fullPath, name string) bool
}

func NewLocalFSService(cfg LocalFSServiceConfig) *LocalFSService {
	return &LocalFSService{
		hostFS:   cfg.HostFS,
		launcher: cfg.Launcher,
		isHidden: cfg.IsHidden,
	}
}

func (s *LocalFSService) DefaultPath() (string, error) {
	if s.hostFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.DefaultPath(), nil
}

func (s *LocalFSService) ResolvePath(path string) (string, error) {
	if s.hostFS == nil {
		return "", fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.ResolvePath(path)
}

func (s *LocalFSService) List(dirPath string, includeHidden bool) ([]domain.LocalFileEntry, error) {
	if s.hostFS == nil {
		return nil, fmt.Errorf("local file service unavailable")
	}
	if dirPath == "" {
		dirPath = s.hostFS.DefaultPath()
	}
	return s.hostFS.List(dirPath, includeHidden, s.isHidden)
}

func (s *LocalFSService) Remove(localPath string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.Remove(localPath)
}

func (s *LocalFSService) Mkdir(dirPath string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.Mkdir(dirPath)
}

func (s *LocalFSService) Rename(oldPath, newPath string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.Rename(oldPath, newPath)
}

func (s *LocalFSService) CreateFile(localPath string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.CreateFile(localPath)
}

// WriteSecretFile writes data to a user-chosen path at owner-only permissions.
func (s *LocalFSService) WriteSecretFile(localPath string, data []byte) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.WriteSecretFile(localPath, data)
}

func (s *LocalFSService) Copy(srcPath, destDir string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	return s.hostFS.Copy(srcPath, destDir)
}

// OpenWithSystem opens a local file with the system default app or editor.
func (s *LocalFSService) OpenWithSystem(localPath, editorPath string) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	if s.launcher == nil {
		return fmt.Errorf("local app launcher unavailable")
	}
	abs, err := s.hostFS.ResolvePath(localPath)
	if err != nil {
		return err
	}
	editorPath = strings.TrimSpace(editorPath)
	if editorPath != "" {
		// A named application receives the file as an argument, so `notepad payload.exe` displays
		// bytes rather than running them. The user chose the program; nothing here decides.
		return s.launcher.OpenWith(editorPath, abs)
	}
	// The default association is the operating system deciding what to do with the file, and for an
	// executable it decides to run it. That turns this method into a way to execute anything the
	// caller can first put on disk - and the caller is the Wails bridge, which a script in the UI
	// reaches. Downloading a file and asking politely for it to be opened should not be a way to
	// run it.
	if domain.IsExecutableForSystemOpen(abs) {
		return fmt.Errorf("%s: %w", filepath.Base(abs), domain.ErrExecutableSystemOpen)
	}
	return s.launcher.OpenDefault(abs)
}

func trimEditorPath(editorPath string) string {
	return strings.TrimSpace(editorPath)
}

func (s *LocalFSService) StartFileWatch(localPath string, onChanged func()) {
	if s.hostFS == nil || onChanged == nil {
		return
	}
	abs, err := s.hostFS.ResolvePath(localPath)
	if err != nil {
		return
	}
	info, err := s.hostFS.Stat(localPath)
	if err != nil {
		return
	}
	initialMod := info.ModTime
	safego.GoNamed("localfs.watch", func() {
		ticker := time.NewTicker(localFSWatchInterval)
		defer ticker.Stop()
		timeout := time.After(localFSWatchTimeout)
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				cur, err := s.hostFS.Stat(abs)
				if err != nil {
					return
				}
				if cur.ModTime.After(initialMod) {
					onChanged()
					return
				}
			}
		}
	})
}
