//go:build !windows

package localshell

import (
	"log/slog"
	"syscall"

	"github.com/aymanbagabas/go-pty"
)

// startupArgs is empty on Unix. A shell attached to a pseudo-terminal already detects that it is
// interactive and reads the user's startup files, and these systems are UTF-8 by default - both
// of the things the Windows path has to arrange explicitly.
func startupArgs(string) []string {
	return nil
}

// configureCmd gives the shell its own process group, which is what makes killing the whole tree
// possible later: a signal sent to the negated group id reaches every descendant, and without
// Setpgid the shell shares the application's group and there would be nothing to aim at.
func configureCmd(cmd *pty.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// processGroup carries no handle on Unix: the group is named by the shell's own pid, so there is
// nothing extra to hold. The type exists so the cross-platform terminal struct has one shape.
type processGroup struct{}

// adopt is a no-op. Setpgid at spawn already established the group.
func (t *terminal) adopt() {}

// killTree signals the shell's whole process group.
//
// The negated pid is the point: SIGKILL to -pid reaches every descendant that has not started a
// group of its own, so closing the tab takes the `ping` and the still-running build with it
// rather than orphaning them.
func (t *terminal) killTree() {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// ESRCH just means the shell already exited and took its group along, which is the
		// ordinary race between the user typing `exit` and the user closing the tab.
		if err != syscall.ESRCH {
			slog.Warn("local terminal: killing process group failed", "err", err)
		}
	}
}
