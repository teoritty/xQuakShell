package guacamole

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// GuacdManager manages the lifecycle of a local guacd process.
type GuacdManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	port    int
	running bool
}

func debugLogPath() string {
	// Resolve relative to repository root for reliability when app is started from other CWDs.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "debug-fb712f.log"
	}
	baseDir := filepath.Dir(file) // .../internal/infra/guacamole
	repoRoot := filepath.Clean(filepath.Join(baseDir, "..", "..", ".."))
	return filepath.Join(repoRoot, "debug-fb712f.log")
}

func appendDebugNDJSON(runID, hypothesisID, location, message string, data map[string]any) {
	payload := map[string]any{
		"id":           fmt.Sprintf("log_%d", time.Now().UnixNano()),
		"timestamp":   time.Now().UnixMilli(),
		"location":    location,
		"message":     message,
		"runId":       runID,
		"hypothesisId": hypothesisID,
		"sessionId":   "fb712f",
	}
	if data != nil {
		payload["data"] = data
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}

	f, err := os.OpenFile(debugLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// NewGuacdManager creates a new guacd process manager.
func NewGuacdManager() *GuacdManager {
	return &GuacdManager{}
}

// EnsureRunning starts guacd if it's not already running and returns the port.
func (m *GuacdManager) EnsureRunning() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	appendDebugNDJSON("pre-fix", "H1_guacd_missing_after_make_or_portable", "guacdproc.go:EnsureRunning:start", "EnsureRunning called", map[string]any{
		"running": m.running,
		"port":    m.port,
	})

	if m.running && m.cmd != nil && m.cmd.Process != nil {
		if m.isAlive() {
			return m.port, nil
		}
		m.running = false
	}

	port, err := findFreePort()
	if err != nil {
		return 0, fmt.Errorf("find free port for guacd: %w", err)
	}

	binary := m.findBinary()
	appendDebugNDJSON("pre-fix", "H2_guacdproc_search_paths_or_filename", "guacdproc.go:EnsureRunning:binary_selected", "guacd binary selected", map[string]any{
		"binary": binary,
		"port":   port,
	})
	if binary == "" {
		return 0, fmt.Errorf("guacd binary not found; install guacd or place it in the portable directory next to the executable")
	}

	cmd := exec.Command(binary, "-b", "127.0.0.1", "-l", strconv.Itoa(port), "-L", "info", "-f")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start guacd (%s): %w", binary, err)
	}

	if err := waitForPort("127.0.0.1", port, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return 0, fmt.Errorf("guacd failed to start listening on port %d: %w", port, err)
	}

	m.cmd = cmd
	m.port = port
	m.running = true

	log.Printf("[guacd] started on 127.0.0.1:%d (pid %d, binary %s)", port, cmd.Process.Pid, binary)

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.running = false
			log.Printf("[guacd] process exited (pid %d)", cmd.Process.Pid)
		}
		m.mu.Unlock()
	}()

	return port, nil
}

// Port returns the port guacd is listening on, or 0 if not running.
func (m *GuacdManager) Port() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return m.port
	}
	return 0
}

// Stop terminates the guacd process.
func (m *GuacdManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil && m.running {
		log.Printf("[guacd] stopping (pid %d)", m.cmd.Process.Pid)
		_ = m.cmd.Process.Kill()
		m.running = false
	}
}

func (m *GuacdManager) isAlive() bool {
	if m.cmd == nil || m.cmd.Process == nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *GuacdManager) findBinary() string {
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "guacd", guacdBinaryName()),
			filepath.Join(exeDir, guacdBinaryName()),
		}
		existence := map[string]bool{}
		for _, c := range candidates {
			_, statErr := os.Stat(c)
			existence[c] = (statErr == nil)
			if _, err := os.Stat(c); err == nil {
				appendDebugNDJSON("pre-fix", "H2_guacdproc_search_paths_or_filename", "guacdproc.go:findBinary:candidate_match", "guacd binary found in candidate paths", map[string]any{
					"candidates": existence,
					"chosen":     c,
				})
				return c
			}
		}
		appendDebugNDJSON("pre-fix", "H2_guacdproc_search_paths_or_filename", "guacdproc.go:findBinary:no_candidate_match", "guacd binary not found in candidate paths", map[string]any{
			"candidates": existence,
			"exeDir":     exeDir,
		})
	}

	if path, err := exec.LookPath(guacdBinaryName()); err == nil {
		appendDebugNDJSON("pre-fix", "H3_guacd_not_in_expected_location_but_in_PATH", "guacdproc.go:findBinary:lookpath_match", "guacd binary found in PATH", map[string]any{
			"path": path,
		})
		return path
	}

	return ""
}

func guacdBinaryName() string {
	if runtime.GOOS == "windows" {
		return "guacd.exe"
	}
	return "guacd"
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func waitForPort(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("%s:%d", host, port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
