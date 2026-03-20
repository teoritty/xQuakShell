//go:build windows

package connectors

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"ssh-client/internal/domain"
)

var (
	modUser32                    = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = modUser32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procShowWindow               = modUser32.NewProc("ShowWindow")
	procIsWindowVisible          = modUser32.NewProc("IsWindowVisible")
)

const swRestore = 9

func buildMstscArgs(conn *domain.Connection, _ string) []string {
	cfg := conn.RDPConfig
	host := cfg.Host
	port := cfg.Port
	if port <= 0 {
		port = 3389
	}

	server := strings.TrimSpace(host)
	if h, pstr, splitErr := net.SplitHostPort(server); splitErr == nil {
		server = h
		if p, e := strconv.Atoi(pstr); e == nil && p > 0 {
			port = p
		}
	}

	args := []string{fmt.Sprintf("/v:%s:%d", server, port)}
	return args
}

// StartExternalRDPProcess launches mstsc.exe and returns the running *exec.Cmd.
// The caller is responsible for calling cmd.Wait() to reap the process.
func StartExternalRDPProcess(conn *domain.Connection, password string) (*exec.Cmd, error) {
	if conn == nil || conn.RDPConfig == nil || conn.RDPConfig.Host == "" {
		return nil, fmt.Errorf("rdp host not configured")
	}
	args := buildMstscArgs(conn, password)
	cmd := exec.Command("mstsc", args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start mstsc: %w", err)
	}
	return cmd, nil
}

// FocusWindowByPID enumerates top-level windows, finds one belonging
// to the given process, restores it if minimized, and brings it to the foreground.
func FocusWindowByPID(pid int) error {
	targetPID := uint32(pid)
	var foundHWND uintptr

	cb := syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID == targetPID {
			visible, _, _ := procIsWindowVisible.Call(hwnd)
			if visible != 0 {
				foundHWND = hwnd
				return 0
			}
		}
		return 1
	})

	procEnumWindows.Call(cb, 0)
	if foundHWND == 0 {
		return fmt.Errorf("no visible window found for PID %d", pid)
	}

	procShowWindow.Call(foundHWND, swRestore)
	procSetForegroundWindow.Call(foundHWND)
	return nil
}

// LaunchExternalRDP launches the external RDP client without tracking.
func LaunchExternalRDP(conn *domain.Connection, password string) error {
	cmd, err := StartExternalRDPProcess(conn, password)
	if err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
