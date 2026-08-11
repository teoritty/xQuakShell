package plugin

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	domainplugin "xquakshell/internal/domain/plugin"
)

// childRequest is everything a platform needs to bring one plugin process up. It is a struct rather
// than a parameter list because the two implementations want different subsets of it and the list
// was already at the limit.
type childRequest struct {
	dataRoot        string
	plugin          domainplugin.InstalledPlugin
	sessionID       string
	entryPath       string
	instanceDataDir string
	env             []string
	// stderr receives the child's standard error. The caller owns it and closes it, including when
	// startPluginChild fails.
	stderr io.WriteCloser
}

// startedChild is a running plugin process and the two pipes the host speaks to it through.
type startedChild struct {
	child  childProcess
	cancel context.CancelFunc
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// startExecChild is the os/exec spawn, used on every platform that is not confining this process
// and on Linux where the confinement rides in the argv rather than in how the process is created.
func startExecChild(target spawnTarget, req childRequest) (startedChild, error) {
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

	// #nosec G204 -- launching a plugin binary is this package's entire purpose. The target is
	// either an entry path resolved from the plugin directory and checksum-verified at install
	// time, or this binary itself with arguments this package built; there is no shell, so neither
	// can expand into another command.
	cmd := exec.CommandContext(procCtx, target.path, target.args...)
	cmd.Env = req.env
	cmd.Stderr = req.stderr
	if err := configurePluginCmd(cmd); err != nil {
		procCancel()
		return startedChild{}, fmt.Errorf("configure plugin process: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		procCancel()
		return startedChild{}, fmt.Errorf("plugin stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		procCancel()
		return startedChild{}, fmt.Errorf("plugin stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		procCancel()
		return startedChild{}, fmt.Errorf("start plugin %s: %w", req.plugin.Manifest.ID, err)
	}
	return startedChild{child: newExecChild(cmd), cancel: procCancel, stdin: stdin, stdout: stdout}, nil
}
