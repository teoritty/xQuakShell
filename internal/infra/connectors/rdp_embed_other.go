//go:build !windows

package connectors

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"ssh-client/internal/domain"
)

// StartExternalRDPProcess launches xfreerdp and returns the running *exec.Cmd.
// The caller is responsible for calling cmd.Wait() to reap the process.
func StartExternalRDPProcess(conn *domain.Connection, password string) (*exec.Cmd, error) {
	if conn == nil || conn.RDPConfig == nil || conn.RDPConfig.Host == "" {
		return nil, fmt.Errorf("rdp host not configured")
	}
	cfg := conn.RDPConfig
	host := strings.TrimSpace(cfg.Host)
	port := cfg.Port
	if port <= 0 {
		port = 3389
	}

	if h, pstr, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
		if p, e := strconv.Atoi(pstr); e == nil && p > 0 {
			port = p
		}
	}

	args := []string{
		"/v:" + fmt.Sprintf("%s:%d", host, port),
		"/cert:ignore",
	}
	if cfg.Username != "" {
		args = append(args, "/u:"+cfg.Username)
	}
	if cfg.Domain != "" {
		args = append(args, "/d:"+cfg.Domain)
	}
	if password != "" {
		args = append(args, "/p:"+password)
	}

	cmd := exec.Command("xfreerdp", args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to launch xfreerdp: %w", err)
	}
	return cmd, nil
}

// FocusWindowByPID is a no-op on non-Windows platforms.
func FocusWindowByPID(_ int) error {
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
