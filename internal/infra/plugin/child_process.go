package plugin

import "os/exec"

// childProcess is the plugin's OS process as everything downstream of the spawn needs it: an
// identity for the bounds that are keyed by pid, one Wait, and a kill.
//
// It exists because the spawn is about to stop being a single code path. A Windows AppContainer
// child cannot be an *exec.Cmd — os/exec exposes no way to pass
// PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES to CreateProcess, so that child has to be built by
// hand from pipes and a STARTUPINFOEX. The reaper, the stop path and the pid tracking have no stake
// in which of the two they were handed, and this is where that stops being visible to them.
type childProcess interface {
	// Pid reports the OS process id. It is valid for the life of the value: a childProcess is only
	// ever constructed around a process that has already started.
	Pid() int

	// Wait blocks until the process exits and reports its exit status. Exactly one caller may ever
	// invoke it — processReaper owns that call, and a second Wait on the same child is an error the
	// OS layer does not diagnose usefully.
	Wait() error

	// Kill terminates this process and nothing else. Tearing down the whole process group is a
	// different operation — killPluginProcess, which sends to -pid on Linux — and it is deliberately
	// not folded in here: the two callers want different blast radii.
	Kill() error
}

// execChild is the os/exec-backed childProcess, and the only implementation until the sandboxed
// Windows spawn lands.
type execChild struct {
	cmd *exec.Cmd
}

func newExecChild(cmd *exec.Cmd) *execChild {
	return &execChild{cmd: cmd}
}

func (c *execChild) Pid() int {
	if c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func (c *execChild) Wait() error {
	return c.cmd.Wait()
}

func (c *execChild) Kill() error {
	if c.cmd.Process == nil {
		return nil
	}
	return c.cmd.Process.Kill()
}
