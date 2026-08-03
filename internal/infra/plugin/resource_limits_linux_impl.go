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

	// No RLIMIT_NPROC here. On Linux it is accounted per-UID, not per-process: with the
	// host, other apps and (on CI) parallel test binaries all under one UID, the user's
	// task count routinely exceeds any small fixed cap, so the plugin's very first
	// clone() fails with EAGAIN and the Go runtime dies before main() with
	// "runtime: failed to create new OS thread" — the host then sees initialize EOF.
	// A runaway thread spawner is still bounded indirectly by RLIMIT_DATA (thread
	// stacks are private anonymous mappings); a real per-tree cap needs a pids cgroup.
	return nil
}
