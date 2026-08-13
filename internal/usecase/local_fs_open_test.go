package usecase

import (
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// recordingLauncher notes which of the two open paths was taken, which is the whole distinction the
// refusal rests on.
type recordingLauncher struct {
	defaultOpened string
	withApp       string
	withFile      string
}

func (l *recordingLauncher) OpenDefault(path string) error {
	l.defaultOpened = path
	return nil
}

func (l *recordingLauncher) OpenWith(appPath, filePath string) error {
	l.withApp, l.withFile = appPath, filePath
	return nil
}

// passthroughHostFS resolves a path to itself, so a test can name the file it means.
type passthroughHostFS struct{ domain.HostFileSystem }

func (passthroughHostFS) ResolvePath(p string) (string, error) { return p, nil }

func newOpenTestService(l domain.HostAppLauncher) *LocalFSService {
	return NewLocalFSService(LocalFSServiceConfig{HostFS: passthroughHostFS{}, Launcher: l})
}

// The chain: anything holding the Wails bridge puts a file on disk and asks for it to be opened.
// The default association is the OS deciding what to do, and for an executable it decides to run
// it - so "open this file" was a way to run arbitrary code as the user.
func TestOpenWithSystemRefusesAnExecutableOnTheDefaultPath(t *testing.T) {
	launcher := &recordingLauncher{}
	svc := newOpenTestService(launcher)

	err := svc.OpenWithSystem(`C:\Users\me\Downloads\payload.exe`, "")

	if !errors.Is(err, domain.ErrExecutableSystemOpen) {
		t.Fatalf("OpenWithSystem err = %v, want ErrExecutableSystemOpen", err)
	}
	if launcher.defaultOpened != "" {
		t.Errorf("the launcher was still asked to open %q", launcher.defaultOpened)
	}
}

// A named application receives the file as an argument: `notepad payload.exe` displays bytes rather
// than running them. The user chose the program, so nothing here second-guesses it - and refusing
// would break opening a binary in a hex editor, which is a real thing people do.
func TestOpenWithSystemStillOpensAnExecutableInANamedEditor(t *testing.T) {
	launcher := &recordingLauncher{}
	svc := newOpenTestService(launcher)

	if err := svc.OpenWithSystem(`C:\tmp\payload.exe`, `C:\Windows\notepad.exe`); err != nil {
		t.Fatalf("OpenWithSystem err = %v, want nil", err)
	}
	if launcher.withApp != `C:\Windows\notepad.exe` || launcher.withFile != `C:\tmp\payload.exe` {
		t.Errorf("OpenWith got (%q, %q), want the editor and the file", launcher.withApp, launcher.withFile)
	}
	if launcher.defaultOpened != "" {
		t.Error("the default association was used despite an editor being named")
	}
}

// The half that keeps the feature: opening a downloaded document in its viewer is the point.
func TestOpenWithSystemOpensAnOrdinaryDocument(t *testing.T) {
	launcher := &recordingLauncher{}
	svc := newOpenTestService(launcher)

	if err := svc.OpenWithSystem(`C:\tmp\report.pdf`, ""); err != nil {
		t.Fatalf("OpenWithSystem err = %v, want nil", err)
	}
	if launcher.defaultOpened != `C:\tmp\report.pdf` {
		t.Errorf("the document was not opened; defaultOpened = %q", launcher.defaultOpened)
	}
}

// The refusal names the file so the user can tell which one was blocked, and does not leak the
// resolved absolute path into the message beyond its base name.
func TestTheRefusalNamesTheFile(t *testing.T) {
	svc := newOpenTestService(&recordingLauncher{})

	err := svc.OpenWithSystem(`C:\Users\me\Downloads\invoice.pdf.exe`, "")

	if err == nil || !strings.Contains(err.Error(), "invoice.pdf.exe") {
		t.Fatalf("err = %v, want it to name invoice.pdf.exe", err)
	}
	if strings.Contains(err.Error(), `C:\Users\me`) {
		t.Errorf("err = %v; it leaks the containing directory", err)
	}
}
