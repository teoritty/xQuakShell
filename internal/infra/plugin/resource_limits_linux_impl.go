//go:build linux

package plugin

import (
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"

	"golang.org/x/sys/unix"
)

func applyLinuxResourceLimits(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid plugin pid")
	}

	// RLIMIT_DATA, not RLIMIT_AS: AS caps *virtual* address space, and the Go runtime
	// reserves multi-GiB PROT_NONE arenas at startup while touching only a few MiB —
	// under a 128 MiB AS cap a Go plugin dies before main() with
	// "fatal error: failed to reserve page summary memory". RLIMIT_DATA (kernel >= 4.7)
	// counts brk plus committed private anonymous mappings, so it caps what the plugin
	// actually allocates and still kills a runaway allocator.
	mem := uint64(domainplugin.MaxPluginProcessMemoryBytes)
	if err := unix.Prlimit(pid, unix.RLIMIT_DATA, &unix.Rlimit{Cur: mem, Max: mem}, nil); err != nil {
		return fmt.Errorf("plugin memory limit: %w", err)
	}

	files := uint64(domainplugin.MaxPluginProcessOpenFiles)
	if err := unix.Prlimit(pid, unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: files, Max: files}, nil); err != nil {
		return fmt.Errorf("plugin open-files limit: %w", err)
	}

	nproc := uint64(domainplugin.MaxPluginProcessThreads)
	if err := unix.Prlimit(pid, unix.RLIMIT_NPROC, &unix.Rlimit{Cur: nproc, Max: nproc}, nil); err != nil {
		return fmt.Errorf("plugin thread limit: %w", err)
	}
	return nil
}
