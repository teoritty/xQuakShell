//go:build !windows

package plugin

import (
	"os"
	"syscall"
)

// processAliveForTest answers whether the OS process is still running. Signal 0 performs the
// permission and existence check without delivering anything, which is the portable POSIX way to ask.
func processAliveForTest(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
