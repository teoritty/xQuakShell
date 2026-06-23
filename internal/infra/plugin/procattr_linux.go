//go:build linux

package plugin

import (
	"os/exec"
	"syscall"
)

func configurePluginCmd(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	return nil
}
