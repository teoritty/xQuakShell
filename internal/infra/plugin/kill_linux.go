//go:build linux

package plugin

import "syscall"

func killPluginProcess(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
