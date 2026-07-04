package logwindow

import (
	"os"
	"runtime"
	"syscall"
	"time"
)

// watchParentExit terminates the viewer when the parent process exits.
func watchParentExit(parentPID int) {
	if parentPID <= 0 {
		return
	}
	go func() {
		if waitProcessExit(parentPID) {
			os.Exit(0)
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !processAlive(parentPID) {
				os.Exit(0)
			}
		}
	}()
}

func processAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		return windowsProcessAlive(pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
