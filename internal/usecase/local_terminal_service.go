package usecase

import (
	"fmt"
	"log/slog"
	"sync"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// LocalTerminalInfo is what the UI needs to draw a tab for a running shell.
type LocalTerminalInfo struct {
	ID    string
	Title string
}

// LocalTerminalPresenter delivers a shell's output and its ending to the UI. One way only: there
// is no path for anything but a keystroke to reach a local shell's input, which is what stops a
// compromised remote host's output from ever becoming a local command.
type LocalTerminalPresenter interface {
	Output(id, dataBase64 string)
	Closed(id string)
}

// LocalTerminalAuditor records that a shell was started or stopped. Keystrokes are deliberately
// not offered here - see LocalTerminalAuditRecorder.
type LocalTerminalAuditor interface {
	Opened(id, shellName string)
	Closed(id, shellName string)
}

// localTerminalHandle is one open shell and the title its tab carries.
type localTerminalHandle struct {
	pty   domain.LocalTerminalPTY
	shell domain.ShellOption
	title string
}

// LocalTerminalServiceConfig collects the collaborators so the constructor stays inside the
// five-parameter budget.
type LocalTerminalServiceConfig struct {
	Factory   domain.LocalTerminalFactory
	Catalog   domain.ShellCatalog
	Presenter LocalTerminalPresenter
	Auditor   LocalTerminalAuditor
	// ShellID reports which shell the user picked in settings. It is read at open time rather
	// than cached so a change in settings applies to the next terminal without any invalidation.
	ShellID func() string
}

// LocalTerminalService owns every open local shell.
//
// It keeps its own registry rather than joining SessionRegistry. A session owns a connection, a
// vault binding and a host key decision; a local shell owns none of those and must never appear
// to. The separation is also what keeps this feature out of the session close path, which is
// where this codebase's session bugs have historically lived.
type LocalTerminalService struct {
	cfg LocalTerminalServiceConfig

	mu   sync.Mutex
	open map[string]*localTerminalHandle
}

// NewLocalTerminalService returns a service with no shells open.
func NewLocalTerminalService(cfg LocalTerminalServiceConfig) *LocalTerminalService {
	return &LocalTerminalService{cfg: cfg, open: make(map[string]*localTerminalHandle)}
}

// ListShells reports the shells available for the settings picker.
func (s *LocalTerminalService) ListShells() []domain.ShellOption {
	if s.cfg.Catalog == nil {
		return nil
	}
	return s.cfg.Catalog.List()
}

// Open starts a shell and returns the tab to draw for it.
//
// Takes no argument naming a program, and there is deliberately nowhere to add one: the shell
// comes from settings by id, and resolving that id happens behind the factory. That absence is
// what makes command injection impossible here rather than merely guarded against.
func (s *LocalTerminalService) Open() (LocalTerminalInfo, error) {
	if s.cfg.Factory == nil {
		return LocalTerminalInfo{}, domain.ErrLocalTerminalUnsupported
	}

	// StartDir is left to the factory, which falls back to the user's home directory. Deciding it
	// here would mean this layer knowing what a home directory is on each platform.
	term, shell, err := s.cfg.Factory.Start(s.storedShellID(), domain.LocalTerminalOptions{
		Cols: defaultTerminalCols,
		Rows: defaultTerminalRows,
	})
	if err != nil {
		return LocalTerminalInfo{}, fmt.Errorf("open local terminal: %w", err)
	}

	id := newLocalTerminalID()
	info := s.register(id, term, shell)

	if s.cfg.Auditor != nil {
		s.cfg.Auditor.Opened(id, shell.Name)
	}
	safego.GoNamed("localterminal.pump", func() {
		pumpLocalTerminalOutput(id, term.Output(), s.emit, func() { s.reap(id) })
	})
	return info, nil
}

// register files a started shell and works out the title its tab shows.
func (s *LocalTerminalService) register(
	id string,
	term domain.LocalTerminalPTY,
	shell domain.ShellOption,
) LocalTerminalInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	title := disambiguateTitle(shell.Name, s.titlesInUse())
	s.open[id] = &localTerminalHandle{pty: term, shell: shell, title: title}
	return LocalTerminalInfo{ID: id, Title: title}
}

// Write forwards keystrokes to a shell.
func (s *LocalTerminalService) Write(id string, data []byte) error {
	handle, ok := s.lookup(id)
	if !ok {
		return domain.ErrLocalTerminalNotFound
	}
	return handle.pty.Write(data)
}

// Resize reports a new window size for a shell.
func (s *LocalTerminalService) Resize(id string, cols, rows uint16) error {
	handle, ok := s.lookup(id)
	if !ok {
		return domain.ErrLocalTerminalNotFound
	}
	return handle.pty.Resize(cols, rows)
}

// Close ends a shell because the user closed its tab.
func (s *LocalTerminalService) Close(id string) error {
	handle, ok := s.lookup(id)
	if !ok {
		// Already gone: the shell exited on its own a moment before the tab was closed. That is
		// an ordinary race, not something to report.
		return nil
	}
	if err := handle.pty.Close(); err != nil {
		slog.Warn("local terminal: close failed", "id", id, "err", err)
	}
	return nil
}

// CloseAll ends every shell. Called when the application shuts down, so that no shell - and
// nothing a shell started - outlives the window it was opened from.
func (s *LocalTerminalService) CloseAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.open))
	for id := range s.open {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		_ = s.Close(id)
	}
}

// reap runs when a shell's output ends, whichever side ended it: the user closing the tab, or the
// shell exiting on its own. It is the single place a terminal leaves the registry, so the tab and
// the audit trail agree regardless of which happened.
func (s *LocalTerminalService) reap(id string) {
	s.mu.Lock()
	handle, ok := s.open[id]
	delete(s.open, id)
	s.mu.Unlock()
	if !ok {
		return
	}

	if err := handle.pty.Close(); err != nil {
		slog.Warn("local terminal: close on reap failed", "id", id, "err", err)
	}
	if s.cfg.Auditor != nil {
		s.cfg.Auditor.Closed(id, handle.shell.Name)
	}
	if s.cfg.Presenter != nil {
		s.cfg.Presenter.Closed(id)
	}
}

func (s *LocalTerminalService) emit(id, dataBase64 string) {
	if s.cfg.Presenter != nil {
		s.cfg.Presenter.Output(id, dataBase64)
	}
}

func (s *LocalTerminalService) lookup(id string) (*localTerminalHandle, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	handle, ok := s.open[id]
	return handle, ok
}

// titlesInUse must be called with the lock held.
func (s *LocalTerminalService) titlesInUse() []string {
	out := make([]string, 0, len(s.open))
	for _, handle := range s.open {
		out = append(out, handle.title)
	}
	return out
}

func (s *LocalTerminalService) storedShellID() string {
	if s.cfg.ShellID == nil {
		return ""
	}
	return s.cfg.ShellID()
}
