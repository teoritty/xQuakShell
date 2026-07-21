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
	reaper *processReaper
	stderr io.WriteCloser
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func spawnPluginProcess(ctx context.Context, dataRoot string, plugin domainplugin.InstalledPlugin, sessionID string) (*spawnedProcess, error) {
	entryPath, err := ResolveEngineEntryPath(plugin.RootDir, plugin.Manifest.Engine.Entry)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin entry: %w", err)
	}
	if err := plugin.Manifest.CompatibleWithCore(domainplugin.HostCoreVersion); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, entryPath)
	cmd.Env = PluginProcessEnv(dataRoot, plugin.Manifest.ID, sessionID)
	stderrLog := NewRedactingStderrWriter(plugin.Manifest.ID)
	cmd.Stderr = stderrLog
	if err := configurePluginCmd(cmd); err != nil {
		_ = stderrLog.Close()
		return nil, fmt.Errorf("configure plugin process: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrLog.Close()
		return nil, fmt.Errorf("plugin stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stderrLog.Close()
		return nil, fmt.Errorf("plugin stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stderrLog.Close()
		return nil, fmt.Errorf("start plugin %s: %w", plugin.Manifest.ID, err)
	}

	reaper := newProcessReaper(cmd)
	reaper.Start()
	return &spawnedProcess{
		cmd:    cmd,
		reaper: reaper,
		stderr: stderrLog,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}
