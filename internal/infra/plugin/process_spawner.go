package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	domainplugin "xquakshell/internal/domain/plugin"
)

// errStartAbortedByStop is returned when a Stop arrived while this Start was still spawning, so the
// process it brought up has been torn down again instead of being published.
var errStartAbortedByStop = errors.New("plugin start aborted by a concurrent stop")

type spawnedProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	reaper *processReaper
	stderr io.WriteCloser
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// spawnPluginProcess starts the plugin binary. It deliberately takes no context: see the comment on
// procCtx below — the caller's context must not own the child process's lifetime, and an unused
// ctx parameter here would be an invitation to wire it back in.
func spawnPluginProcess(dataRoot string, plugin domainplugin.InstalledPlugin, sessionID string) (*spawnedProcess, error) {
	entryPath, err := ResolveEngineEntryPath(plugin.RootDir, plugin.Manifest.Engine.Entry)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin entry: %w", err)
	}
	// The child process is deliberately NOT tied to the caller's context. exec.CommandContext makes
	// the passed context own the LIFETIME of the child: cancelling it kills the process. Every caller
	// of Start passes a short-lived request context (a WithTimeout with a `defer cancel()`), so a
	// plugin used to die the moment the call that started it returned — including a supervisor
	// restart, which cancelled on its own success path. A plugin process outlives the operation that
	// started it by definition; only Stop/StopAll/crash teardown may end it.
	//
	// The caller's context still bounds the START OPERATION — initializePluginProcess(ctx, …) in
	// Start keeps using it for the handshake, and a cancellation there fails the start, whose deferred
	// teardown kills the process explicitly via closeResources(true).
	//
	// procCancel is handed to the owner (managedProcess) and fired from closeResources so the
	// context and its watchdog goroutine are released when the process is gone.
	procCtx, procCancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, entryPath)
	cmd.Env = PluginProcessEnv(dataRoot, plugin.Manifest.ID, sessionID)
	stderrLog := NewRedactingStderrWriter(plugin.Manifest.ID)
	cmd.Stderr = stderrLog
	if err := configurePluginCmd(cmd); err != nil {
		procCancel()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("configure plugin process: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		procCancel()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("plugin stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		procCancel()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("plugin stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		procCancel()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("start plugin %s: %w", plugin.Manifest.ID, err)
	}

	reaper := newProcessReaper(cmd)
	reaper.Start()
	return &spawnedProcess{
		cmd:    cmd,
		cancel: procCancel,
		reaper: reaper,
		stderr: stderrLog,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

// discardSpawnedProcess tears down a child that Start spawned but will not keep. It is the teardown
// for a process that never reached managedProcess, so nothing else can reach it: closeResources
// works from mp's fields and would find them nil.
//
// The kill goes through the reaper because the reaper also waits, which is what turns "signalled"
// into "gone" — and on Windows what makes the pid safe to observe. The job handle is closed here
// too: leaving it open would keep the process alive under KILL_ON_JOB_CLOSE and leak the handle.
func discardSpawnedProcess(spawned *spawnedProcess, job pluginJob) {
	if spawned == nil {
		return
	}
	if spawned.reaper != nil {
		_ = spawned.reaper.Kill()
	}
	if spawned.cancel != nil {
		spawned.cancel()
	}
	if spawned.stderr != nil {
		_ = spawned.stderr.Close()
	}
	closePluginJob(job)
	if spawned.cmd != nil && spawned.cmd.Process != nil {
		untrackPluginPID(spawned.cmd.Process.Pid)
	}
}
