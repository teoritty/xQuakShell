//go:build windows

package plugin

import (
	"fmt"
	"os"

	domainplugin "xquakshell/internal/domain/plugin"

	"xquakshell/internal/infra/plugin/sandbox"
)

// startPluginChild spawns the plugin inside an AppContainer, or through os/exec on a build that
// cannot make one.
//
// The two are separate paths rather than one path with a flag, and the separation is the point: an
// AppContainer child cannot be an *exec.Cmd at all. os/exec exposes Token and no attribute list, so
// PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES has no way through it and the child has to be built by
// hand from pipes and a STARTUPINFOEX.
//
// A platform that cannot confine falls back; a platform that can and then fails does not. The error
// is returned and the start fails, because exec'ing unconfined after asking for confinement is how a
// sandbox stops working for a fraction of users with nobody noticing.
func startPluginChild(req childRequest) (startedChild, error) {
	support := sandbox.Support()
	if !support.Available {
		target, err := resolveSpawnTarget(req.dataRoot, req.plugin, req.entryPath, req.instanceDataDir)
		if err != nil {
			return startedChild{}, err
		}
		return startExecChild(target, req, support.Mode())
	}

	started, err := startContainedChild(req, support)
	if err != nil {
		return fallBackOrRefuse(req, err)
	}
	return started, nil
}

// startContainedChild is the confined path on its own, so that every way it can fail arrives at one
// place and gets the same answer from the policy.
func startContainedChild(req childRequest, support domainplugin.SandboxSupport) (startedChild, error) {
	image, err := resolvePluginImage(req.entryPath)
	if err != nil {
		return startedChild{}, err
	}
	// Creating the profile and starting the process inside it are one step as far as any other
	// goroutine is concerned; see containerMu. A teardown that deleted the profile between them
	// would fail this spawn with an error naming a file that is exactly where it should be.
	containerMu.Lock()
	defer containerMu.Unlock()

	container, err := prepareContainer(req.plugin, req.dataRoot, req.sessionID, req.instanceDataDir)
	if err != nil {
		return startedChild{}, err
	}
	process, err := sandbox.Spawn(container, sandbox.SpawnRequest{
		Exe:    image,
		Env:    sandbox.ContainerEnv(req.env, req.instanceDataDir),
		Stderr: req.stderr,
	})
	if err != nil {
		return startedChild{}, fmt.Errorf("start plugin %s in its app container: %w", req.plugin.Manifest.ID, err)
	}
	return startedChild{
		mode:  support.Mode(),
		child: process,
		// Nothing here is tied to a context. The exec path needs a cancel to release
		// CommandContext's watchdog goroutine; this process has no such watchdog, and its only end
		// is the reaper's kill or its own exit.
		cancel: func() {},
		stdin:  process.Stdin,
		stdout: process.Stdout,
	}, nil
}

// resolvePluginImage turns the manifest's entry path into the file that actually exists.
//
// A manifest names its entry without an extension and the installed binary is `<entry>.exe`.
// Nothing noticed while os/exec did the spawn: CreateProcess supplies a default extension when it
// parses a command line, so the extensionless path worked by accident. lpApplicationName gets no
// such courtesy — it is documented to assume no extension — and the failure it produces is
// ERROR_FILE_NOT_FOUND, which reads as "the plugin is not installed" rather than as "the path was
// spelled short".
//
// Discovery already rewrites the entry for the plugins it finds, so in practice the first branch
// wins. Depending on that would make the spawn silently require a step that happens somewhere else,
// which is the kind of coupling that survives right up until someone builds an InstalledPlugin by
// hand.
func resolvePluginImage(entryPath string) (string, error) {
	if _, err := os.Stat(entryPath); err == nil {
		return entryPath, nil
	}
	withExe := entryPath + ".exe"
	if _, err := os.Stat(withExe); err == nil {
		return withExe, nil
	}
	return "", fmt.Errorf("plugin binary not found at %s or %s", entryPath, withExe)
}
