package logwindow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// SettingsSaver persists debug log window preference changes.
type SettingsSaver interface {
	GetSettings() (domain.AppSettings, error)
	SaveSettings(ctx context.Context, settings domain.AppSettings) error
}

// ChangeNotifier emits debug log window enabled state to the UI.
type ChangeNotifier func(enabled bool)

// Manager owns the log viewer child process and TCP stream server.
type Manager struct {
	stream   domain.LogStream
	settings SettingsSaver
	notify   ChangeNotifier

	mu           sync.Mutex
	listener     net.Listener
	cmd          *exec.Cmd
	shuttingDown bool
	running      bool
}

// NewManager creates a log window manager.
func NewManager(stream domain.LogStream, settings SettingsSaver, notify ChangeNotifier) *Manager {
	if stream == nil {
		panic("logwindow: LogStream is required")
	}
	return &Manager{stream: stream, settings: settings, notify: notify}
}

// SyncEnabled starts or stops the log viewer to match the desired state.
func (m *Manager) SyncEnabled(ctx context.Context, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if enabled {
		m.startLocked(ctx)
		return
	}
	m.stopLocked()
}

// Stop closes the stream and terminates the child process.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	closeJob()
}

func (m *Manager) startLocked(ctx context.Context) {
	if m.running {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		slog.Error("debug log window listen failed", "err", err)
		return
	}
	m.listener = ln
	m.shuttingDown = false

	safego.GoNamed("logwindow.accept", func() { m.acceptLoop(ln) })

	exe, err := os.Executable()
	if err != nil {
		slog.Error("debug log window executable path failed", "err", err)
		_ = ln.Close()
		m.listener = nil
		return
	}

	addr := ln.Addr().String()
	args := []string{
		flagLogViewer,
		flagAddr + addr,
		fmt.Sprintf("%s%d", flagParentPID, os.Getpid()),
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if err := cmd.Start(); err != nil {
		slog.Error("debug log window spawn failed", "err", err)
		_ = ln.Close()
		m.listener = nil
		return
	}
	assignProcessToJob(cmd)
	m.cmd = cmd
	m.running = true

	safego.GoNamed("logwindow.watchChild", func() { m.watchChild(cmd) })
	_ = ctx
}

func (m *Manager) stopLocked() {
	m.shuttingDown = true
	if m.listener != nil {
		_ = m.listener.Close()
		m.listener = nil
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
	m.running = false
}

func (m *Manager) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		safego.GoNamed("logwindow.serveClient", func() { m.serveClient(conn) })
	}
}

func (m *Manager) serveClient(conn net.Conn) {
	defer conn.Close()
	id, backlog, live := m.stream.Subscribe(128)
	defer m.stream.Unsubscribe(id)

	for _, entry := range backlog {
		if err := writeEntry(conn, entry); err != nil {
			return
		}
	}
	for entry := range live {
		if err := writeEntry(conn, entry); err != nil {
			return
		}
	}
}

func writeEntry(w io.Writer, entry domain.DebugLogEntry) error {
	line, err := EncodeLine(entry)
	if err != nil {
		return err
	}
	_, err = w.Write(line)
	return err
}

func (m *Manager) watchChild(cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != cmd {
		return
	}
	m.cmd = nil
	m.running = false
	if m.listener != nil {
		_ = m.listener.Close()
		m.listener = nil
	}
	if m.shuttingDown {
		return
	}
	m.disableSettingAsync()
	if err != nil {
		slog.Debug("debug log viewer exited", "err", err)
	}
}

func (m *Manager) disableSettingAsync() {
	safego.GoNamed("logwindow.disableSetting", func() {
		if m.settings == nil {
			if m.notify != nil {
				m.notify(false)
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		settings, err := m.settings.GetSettings()
		if err != nil {
			slog.Error("debug log window disable settings read failed", "err", err)
			return
		}
		if !settings.Debug.LogWindowEnabled {
			return
		}
		settings.Debug.LogWindowEnabled = false
		if err := m.settings.SaveSettings(ctx, settings); err != nil {
			slog.Error("debug log window disable settings save failed", "err", err)
			return
		}
		if m.notify != nil {
			m.notify(false)
		}
	})
}

// ReadStream connects to the parent stream address and invokes handler for each entry.
func ReadStream(ctx context.Context, addr string, handler func(domain.DebugLogEntry)) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		entry, err := DecodeLine(line)
		if err != nil {
			continue
		}
		handler(entry)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return scanner.Err()
}
