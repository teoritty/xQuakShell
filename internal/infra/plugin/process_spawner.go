package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"

	domainplugin "xquakshell/internal/domain/plugin"
)

// errStartAbortedByStop is returned when a Stop arrived while this Start was still spawning, so the
// process it brought up has been torn down again instead of being published.
var errStartAbortedByStop = errors.New("plugin start aborted by a concurrent stop")

type spawnedProcess struct {
	// sandbox is the boundary this process actually came up behind, decided by the platform that
	// created it rather than assumed from what the build can do.
	sandbox domainplugin.SandboxMode
	// limitsApplied says the child capped its own resources on the way in.
	limitsApplied bool
	// dataRoot is where this instance's directories and its durable permissions were rooted. The
	// teardown needs it to take those permissions back and is reached from places that have no
	// access to the host's configuration, so it travels with the process that they were written for.
	dataRoot string
	child    childProcess
	cancel   context.CancelFunc
	reaper   *processReaper
	stderr   io.WriteCloser
	stdin    io.WriteCloser
	stdout   io.ReadCloser
}

// spawnPluginProcess brings up the plugin binary together with the directories it is allowed to
// write, and returns the instance data directory it prepared. The directories are created here
// rather than by the caller because the temp one's path goes into the child's environment: it has
// to exist before the child that is already being told about it.
//
// It deliberately takes no context: see the comment on procCtx below — the caller's context must
// not own the child process's lifetime, and an unused ctx parameter here would be an invitation to
// wire it back in.
func spawnPluginProcess(dataRoot string, plugin domainplugin.InstalledPlugin, sessionID string, policy domainplugin.SandboxPolicy) (*spawnedProcess, string, error) {
	entryPath, err := ResolveEngineEntryPath(plugin.RootDir, plugin.Manifest.Engine.Entry)
	if err != nil {
		return nil, "", fmt.Errorf("resolve plugin entry: %w", err)
	}
	instanceDataDir, err := preparePluginInstanceDirs(dataRoot, plugin, sessionID)
	if err != nil {
		return nil, "", err
	}
	// Repairs installs written before CopyBundle preserved the execute bit. Without this the fix
	// only helps plugins installed after the update, and every plugin already on a Linux disk stays
	// dead with no hint that reinstalling is what would revive it.
	if err := EnsureEntryExecutable(entryPath); err != nil {
		return nil, "", err
	}
	// How the process is created is the platform's business from here. On Linux it is an ordinary
	// exec of the sandbox shim, which narrows itself and becomes the plugin; on Windows with an
	// AppContainer it is a hand-built CreateProcessW, because os/exec cannot pass the security
	// capabilities that make a container a container. Everything downstream — the pid, the pipes,
	// the reaper, the job — is the same either way, which is what the childProcess seam is for.
	stderrLog := NewRedactingStderrWriter(plugin.Manifest.ID)
	started, err := startPluginChild(childRequest{
		dataRoot:        dataRoot,
		plugin:          plugin,
		sessionID:       sessionID,
		entryPath:       entryPath,
		instanceDataDir: instanceDataDir,
		env:             PluginProcessEnv(instanceDataDir, plugin.Manifest.ID, sessionID),
		stderr:          stderrLog,
		policy:          policy,
	})
	if err != nil {
		_ = stderrLog.Close()
		return nil, "", err
	}

	reaper := newProcessReaper(started.child)
	reaper.Start()
	return &spawnedProcess{
		sandbox:       started.mode,
		limitsApplied: started.limitsApplied,
		dataRoot:      dataRoot,
		child:         started.child,
		cancel:        started.cancel,
		reaper:        reaper,
		stderr:        stderrLog,
		stdin:         started.stdin,
		stdout:        started.stdout,
	}, instanceDataDir, nil
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
	if spawned.child != nil {
		untrackPluginPID(spawned.child.Pid())
	}
}
