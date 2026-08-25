package usecase

import (
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

var (
	_ domain.LocalTerminalFactory = (*fakeShellFactory)(nil)
	_ domain.LocalTerminalPTY     = (*fakeShellPTY)(nil)
	_ LocalTerminalPresenter      = (*fakeTerminalPresenter)(nil)
	_ LocalTerminalAuditor        = (*fakeTerminalAuditor)(nil)
)

type fakeShellPTY struct {
	mu     sync.Mutex
	out    chan []byte
	writes [][]byte
	sizes  [][2]uint16
	closes int
}

func newFakeShellPTY() *fakeShellPTY {
	return &fakeShellPTY{out: make(chan []byte, 8)}
}

func (p *fakeShellPTY) Output() <-chan []byte { return p.out }

func (p *fakeShellPTY) Write(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	return nil
}

func (p *fakeShellPTY) Resize(cols, rows uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sizes = append(p.sizes, [2]uint16{cols, rows})
	return nil
}

// Close is idempotent because the real one is: the tab closing and the shell exiting both reach
// it, and the test would otherwise panic on the second close of the channel.
func (p *fakeShellPTY) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closes++
	if p.closes == 1 {
		close(p.out)
	}
	return nil
}

func (p *fakeShellPTY) closeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closes
}

type fakeShellFactory struct {
	mu        sync.Mutex
	requested []string
	shell     domain.ShellOption
	started   []*fakeShellPTY
	err       error
	lastOpts  domain.LocalTerminalOptions
}

func (f *fakeShellFactory) Start(
	shellID string,
	opts domain.LocalTerminalOptions,
) (domain.LocalTerminalPTY, domain.ShellOption, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, shellID)
	f.lastOpts = opts
	if f.err != nil {
		return nil, domain.ShellOption{}, f.err
	}
	pty := newFakeShellPTY()
	f.started = append(f.started, pty)
	return pty, f.shell, nil
}

type fakeTerminalPresenter struct {
	mu     sync.Mutex
	output []string
	closed []string
}

func (p *fakeTerminalPresenter) Output(_, dataBase64 string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output = append(p.output, dataBase64)
}

func (p *fakeTerminalPresenter) Closed(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, id)
}

func (p *fakeTerminalPresenter) closedIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.closed...)
}

type fakeTerminalAuditor struct {
	mu     sync.Mutex
	events []string
}

func (a *fakeTerminalAuditor) Opened(_, shellName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, "open:"+shellName)
}

func (a *fakeTerminalAuditor) Closed(_, shellName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, "close:"+shellName)
}

func (a *fakeTerminalAuditor) recorded() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.events...)
}

func newTestService(f *fakeShellFactory) (*LocalTerminalService, *fakeTerminalPresenter, *fakeTerminalAuditor) {
	presenter := &fakeTerminalPresenter{}
	auditor := &fakeTerminalAuditor{}
	svc := NewLocalTerminalService(LocalTerminalServiceConfig{
		Factory:   f,
		Presenter: presenter,
		Auditor:   auditor,
		ShellID:   func() string { return "stored-id" },
		HomeDir:   func() string { return "/home/tester" },
	})
	return svc, presenter, auditor
}

// openIDs reads the registry directly. The test lives in the same package precisely so this needs
// no production accessor written for a test's benefit.
func openIDs(s *LocalTerminalService) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.open))
	for id := range s.open {
		out = append(out, id)
	}
	return out
}

func decodeBase64ForTest(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	return string(raw), err
}

// waitFor polls a condition instead of sleeping a fixed time: the pump runs on its own goroutine
// and a fixed sleep is either slow when it passes or flaky when it does not.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestOpenPassesTheStoredShellIDAndHomeDirectory(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, _, _ := newTestService(factory)

	if _, err := svc.Open(); err != nil {
		t.Fatalf("Open() = %v, want nil", err)
	}

	if got := factory.requested[0]; got != "stored-id" {
		t.Errorf("factory asked for %q, want the id from settings", got)
	}
	if got := factory.lastOpts.StartDir; got != "/home/tester" {
		t.Errorf("StartDir = %q, want the home directory", got)
	}
	if factory.lastOpts.Cols == 0 || factory.lastOpts.Rows == 0 {
		t.Error("opened with a zero geometry; the first prompt would draw against no window")
	}
}

func TestOpenReportsTheShellTheFactoryActuallyStarted(t *testing.T) {
	// The user picked pwsh and uninstalled it; the factory fell back. The tab must name what is
	// running, not what was asked for, or the title lies about which shell you are typing into.
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, _, _ := newTestService(factory)

	info, err := svc.Open()
	if err != nil {
		t.Fatalf("Open() = %v, want nil", err)
	}
	if info.Title != "bash" {
		t.Errorf("Title = %q, want bash - the shell that was actually started", info.Title)
	}
}

func TestOpenNumbersRepeatedShellsFromTwo(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "pwsh", Name: "pwsh"}}
	svc, _, _ := newTestService(factory)

	first, _ := svc.Open()
	second, _ := svc.Open()
	third, _ := svc.Open()

	if first.Title != "pwsh" {
		t.Errorf("first title = %q, want the bare name while it is free", first.Title)
	}
	if second.Title != "pwsh 2" || third.Title != "pwsh 3" {
		t.Errorf("titles = %q, %q; want pwsh 2 and pwsh 3", second.Title, third.Title)
	}
	if first.ID == second.ID {
		t.Error("two terminals share an id; a close would hit the wrong one")
	}
}

func TestOpenReusesATitleFreedByAClosedTab(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "zsh", Name: "zsh"}}
	svc, _, _ := newTestService(factory)

	_, _ = svc.Open()
	second, _ := svc.Open()
	_, _ = svc.Open()

	if err := svc.Close(second.ID); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	waitFor(t, "the closed terminal to be reaped", func() bool {
		return len(openIDs(svc)) == 2
	})

	fourth, _ := svc.Open()
	if fourth.Title != "zsh 2" {
		t.Errorf("title = %q, want zsh 2 reused; numbering must fill gaps, not climb", fourth.Title)
	}
}

func TestOpenSurfacesAnUnsupportedSystem(t *testing.T) {
	factory := &fakeShellFactory{err: domain.ErrLocalTerminalUnsupported}
	svc, _, _ := newTestService(factory)

	_, err := svc.Open()
	if !errors.Is(err, domain.ErrLocalTerminalUnsupported) {
		t.Errorf("Open() = %v, want ErrLocalTerminalUnsupported to survive wrapping", err)
	}
}

func TestWriteAndResizeReachTheShell(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, _, _ := newTestService(factory)
	info, _ := svc.Open()

	if err := svc.Write(info.ID, []byte("ls\r")); err != nil {
		t.Fatalf("Write() = %v, want nil", err)
	}
	if err := svc.Resize(info.ID, 120, 40); err != nil {
		t.Fatalf("Resize() = %v, want nil", err)
	}

	pty := factory.started[0]
	pty.mu.Lock()
	defer pty.mu.Unlock()
	if len(pty.writes) != 1 || string(pty.writes[0]) != "ls\r" {
		t.Errorf("writes = %q, want the keystrokes verbatim", pty.writes)
	}
	if len(pty.sizes) != 1 || pty.sizes[0] != [2]uint16{120, 40} {
		t.Errorf("sizes = %v, want one resize to 120x40", pty.sizes)
	}
}

func TestWriteToAnUnknownIDIsRefused(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, _, _ := newTestService(factory)

	if err := svc.Write("lt-nope", []byte("x")); !errors.Is(err, domain.ErrLocalTerminalNotFound) {
		t.Errorf("Write(unknown) = %v, want ErrLocalTerminalNotFound", err)
	}
	if err := svc.Resize("lt-nope", 10, 10); !errors.Is(err, domain.ErrLocalTerminalNotFound) {
		t.Errorf("Resize(unknown) = %v, want ErrLocalTerminalNotFound", err)
	}
}

func TestOutputIsBatchedAndForwarded(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, presenter, _ := newTestService(factory)
	info, _ := svc.Open()

	pty := factory.started[0]
	pty.out <- []byte("hello ")
	pty.out <- []byte("world")

	waitFor(t, "output to reach the presenter", func() bool {
		presenter.mu.Lock()
		defer presenter.mu.Unlock()
		return len(presenter.output) > 0
	})

	presenter.mu.Lock()
	joined := ""
	for _, chunk := range presenter.output {
		decoded, err := decodeBase64ForTest(chunk)
		if err != nil {
			t.Fatalf("presenter got invalid base64: %v", err)
		}
		joined += decoded
	}
	presenter.mu.Unlock()

	if joined != "hello world" {
		t.Errorf("forwarded %q, want the bytes intact across batching", joined)
	}
	_ = info
}

func TestShellExitingOnItsOwnClosesTheTabAndAudits(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, presenter, auditor := newTestService(factory)
	info, _ := svc.Open()

	// The user typed `exit`: the pty ends its output, nobody closed the tab.
	_ = factory.started[0].Close()

	waitFor(t, "the tab to be closed", func() bool { return len(presenter.closedIDs()) == 1 })

	if got := presenter.closedIDs()[0]; got != info.ID {
		t.Errorf("closed %q, want %q", got, info.ID)
	}
	want := []string{"open:bash", "close:bash"}
	if got := auditor.recorded(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

func TestCloseAllEndsEveryShell(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, presenter, _ := newTestService(factory)
	_, _ = svc.Open()
	_, _ = svc.Open()

	svc.CloseAll()

	waitFor(t, "both terminals to close", func() bool { return len(presenter.closedIDs()) == 2 })
	for i, pty := range factory.started {
		if pty.closeCount() == 0 {
			t.Errorf("shell %d was never closed; it would outlive the window", i)
		}
	}
}

func TestCloseOfAnAlreadyGoneTerminalIsNotAnError(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, _, _ := newTestService(factory)

	// The shell exited a moment before the user clicked the tab's close button. That race is
	// ordinary and must not surface as a failure.
	if err := svc.Close("lt-already-gone"); err != nil {
		t.Errorf("Close(gone) = %v, want nil", err)
	}
}

// TestNoVaultLockHookExists guards the decision that a local terminal survives a vault lock.
//
// It asserts an absence, which is the only way to test one: the service must expose nothing a
// lock handler could call, so wiring one up later is a visible API addition rather than a quiet
// behaviour change that kills a user's running build when the vault auto-locks.
func TestNoVaultLockHookExists(t *testing.T) {
	factory := &fakeShellFactory{shell: domain.ShellOption{ID: "bash", Name: "bash"}}
	svc, presenter, _ := newTestService(factory)
	_, _ = svc.Open()

	var locker interface{ OnVaultLocked() }
	if _, ok := any(svc).(interface{ OnVaultLocked() }); ok {
		t.Fatal("LocalTerminalService grew an OnVaultLocked hook; a lock must not kill a shell")
	}
	_ = locker

	if len(presenter.closedIDs()) != 0 {
		t.Error("a terminal closed without anyone asking")
	}
}
