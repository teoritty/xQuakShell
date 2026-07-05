package logwindow

import (
	"os"
	"runtime"
	"syscall"
	"time"

	"ssh-client/internal/pkg/safego"
)

// watchParentExit terminates the viewer when the parent process exits.
func watchParentExit(parentPID int) {
	if parentPID <= 0 {
		return
	}
	safego.GoNamed("logwindow.viewerParent", func() {
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
	})
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
