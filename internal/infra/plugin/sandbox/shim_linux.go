//go:build linux

package sandbox

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// RunShim confines this process to what its argv describes and then becomes the plugin binary. It
// never returns.
//
// There is deliberately no unrestricted path out of here. Every failure exits without exec'ing
// anything, and the absence of a fall-through branch is the entire safety argument for routing a
// plugin through a shim: one `if err != nil { exec anyway }` would turn every kernel quirk, every
// unreadable directory and every argv mistake into a silently unconfined plugin, reported to the
// user as sandboxed because the host asked for a sandbox and the process came up.
//
// Failing here is a plugin that does not start, which is loud, diagnosable from the stderr line
// below (the host's redacting stderr reader forwards it into the log), and recoverable by turning
// the sandbox off. That is the trade this shim makes.
func RunShim(argv []string) {
	args, err := ParseShimArgs(argv)
	if err != nil {
		shimFail(err)
	}
	abi, err := landlockABI()
	if err != nil {
		shimFail(fmt.Errorf("probe landlock: %w", err))
	}
	if abi < abiFilesystem {
		shimFail(fmt.Errorf("kernel reports landlock ABI %d, which cannot confine a filesystem", abi))
	}
	if err := applyLandlock(abi, args); err != nil {
		shimFail(err)
	}

	// Landlock domains only ever narrow across execve, so nothing the plugin does after this point
	// can widen what was just applied — including running this shim again. The environment is
	// passed through unchanged: it is the one the host built for the plugin, and this process only
	// ever existed to stand between the two.
	shimFail(fmt.Errorf("exec %s: %w", args.Exec, unix.Exec(args.Exec, []string{args.Exec}, os.Environ())))
}

func shimFail(err error) {
	fmt.Fprintf(os.Stderr, "plugin sandbox: %v\n", err)
	os.Exit(1)
}
