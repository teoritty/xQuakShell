//go:build linux

package plugin

import (
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
	"golang.org/x/sys/unix"
)

func applyLinuxResourceLimits(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid plugin pid")
	}

	mem := domainplugin.MaxPluginProcessMemoryBytes
	if err := unix.Prlimit(pid, unix.RLIMIT_AS, &unix.Rlimit{Cur: mem, Max: mem}, nil); err != nil {
		return fmt.Errorf("plugin memory limit: %w", err)
	}

	files := domainplugin.MaxPluginProcessOpenFiles
	if err := unix.Prlimit(pid, unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: files, Max: files}, nil); err != nil {
		return fmt.Errorf("plugin open-files limit: %w", err)
	}

	nproc := domainplugin.MaxPluginProcessThreads
	if err := unix.Prlimit(pid, unix.RLIMIT_NPROC, &unix.Rlimit{Cur: nproc, Max: nproc}, nil); err != nil {
		return nil
	}
	return nil
}
