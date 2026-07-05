package usecase

import (
	"errors"
	"testing"
	"time"

	"ssh-client/internal/domain"
)

type mockHostFS struct {
	defaultPath string
	listFn      func(dirPath string, includeHidden bool, isHidden func(string, string) bool) ([]domain.LocalFileEntry, error)
	statFn      func(localPath string) (domain.HostFileInfo, error)
	resolveFn   func(path string) (string, error)
}

func (m *mockHostFS) DefaultPath() string { return m.defaultPath }
func (m *mockHostFS) ResolvePath(path string) (string, error) {
	if m.resolveFn != nil {
		return m.resolveFn(path)
	}
	return path, nil
}
func (m *mockHostFS) Stat(localPath string) (domain.HostFileInfo, error) {
	if m.statFn != nil {
		return m.statFn(localPath)
	}
	return domain.HostFileInfo{}, errors.New("stat")
}
func (m *mockHostFS) List(dirPath string, includeHidden bool, isHidden func(string, string) bool) ([]domain.LocalFileEntry, error) {
	if m.listFn != nil {
		return m.listFn(dirPath, includeHidden, isHidden)
	}
	return nil, nil
}
func (m *mockHostFS) Remove(string) error   { return nil }
func (m *mockHostFS) Mkdir(string) error    { return nil }
func (m *mockHostFS) Rename(_, _ string) error { return nil }
func (m *mockHostFS) CreateFile(string) error { return nil }

type mockLauncher struct {
	openDefault func(path string) error
	openWith    func(appPath, filePath string) error
}

func (m *mockLauncher) OpenDefault(path string) error {
	if m.openDefault != nil {
		return m.openDefault(path)
	}
	return nil
}
func (m *mockLauncher) OpenWith(appPath, filePath string) error {
	if m.openWith != nil {
		return m.openWith(appPath, filePath)
	}
	return nil
}

func TestLocalFSServiceListDefaultPath(t *testing.T) {
	svc := NewLocalFSService(LocalFSServiceConfig{
		HostFS: &mockHostFS{
			defaultPath: "/home",
			listFn: func(dirPath string, _ bool, _ func(string, string) bool) ([]domain.LocalFileEntry, error) {
				if dirPath != "/home" {
					t.Fatalf("dirPath=%q", dirPath)
				}
				return []domain.LocalFileEntry{{Name: "a.txt"}}, nil
			},
		},
	})
	entries, err := svc.List("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestLocalFSServiceOpenWithSystemEditor(t *testing.T) {
	var openedWith string
	svc := NewLocalFSService(LocalFSServiceConfig{
		HostFS: &mockHostFS{
			resolveFn: func(path string) (string, error) { return "/abs/" + path, nil },
		},
		Launcher: &mockLauncher{
			openWith: func(appPath, filePath string) error {
				openedWith = appPath + " " + filePath
				return nil
			},
		},
	})
	if err := svc.OpenWithSystem("file.txt", "  /bin/editor  "); err != nil {
		t.Fatal(err)
	}
	if openedWith != "/bin/editor /abs/file.txt" {
		t.Fatalf("got %q", openedWith)
	}
}

func TestLocalFSServiceStartFileWatch(t *testing.T) {
	start := time.Now()
	calls := 0
	svc := NewLocalFSService(LocalFSServiceConfig{
		HostFS: &mockHostFS{
			resolveFn: func(path string) (string, error) { return path, nil },
			statFn: func(_ string) (domain.HostFileInfo, error) {
				calls++
				mod := start
				if calls >= 2 {
					mod = start.Add(time.Second)
				}
				return domain.HostFileInfo{ModTime: mod}, nil
			},
		},
	})
	changed := make(chan struct{}, 1)
	svc.StartFileWatch("/tmp/file", func() { changed <- struct{}{} })
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not fire")
	}
}

func TestTrimEditorPath(t *testing.T) {
	if got := trimEditorPath("  /bin/vim  "); got != "/bin/vim" {
		t.Fatalf("got %q", got)
	}
}
