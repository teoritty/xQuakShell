//go:build windows

package plugin

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

func configurePluginCmd(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	return nil
}

// JobObjectAvailable reports whether per-process job objects can be created.
func JobObjectAvailable() bool {
	job, err := createPluginJob()
	if err != nil {
		return false
	}
	closePluginJob(job)
	return true
}
