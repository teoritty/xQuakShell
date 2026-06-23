//go:build !windows && !linux

package plugin

import (
	"os/exec"
	"runtime"
	"syscall"
)

// configurePluginCmd sets process-group isolation for plugin children.
//
// Linux uses PR_SET_PDEATHSIG via Pdeathsig (see procattr_linux.go).
// macOS sets Pdeathsig when supported by the platform libc.
// Other Unix targets rely on process-group kill during host shutdown; there is no portable
// kill-on-parent-exit equivalent to Windows Job Objects on every BSD derivative.
func configurePluginCmd(cmd *exec.Cmd) error {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if runtime.GOOS == "darwin" {
		attr.Pdeathsig = syscall.SIGKILL
	}
	cmd.SysProcAttr = attr
	return nil
}
