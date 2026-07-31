package plugin

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	domainplugin "xquakshell/internal/domain/plugin"
)

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
	if err := plugin.Manifest.CompatibleWithCore(domainplugin.HostCoreVersion); err != nil {
		return nil, err
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
