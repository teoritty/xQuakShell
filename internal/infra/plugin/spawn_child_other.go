//go:build !windows

package plugin

import (
	"xquakshell/internal/infra/plugin/sandbox"
)

// startPluginChild spawns through os/exec, which is the whole story everywhere but Windows.
//
// On Linux the confinement is already in the target: resolveSpawnTarget names the sandbox shim and
// puts the plugin binary in its arguments, so the process created here narrows itself and then
// becomes the plugin. Nothing about how it is created has to change, which is exactly why the
// Landlock design was worth preferring.
func startPluginChild(req childRequest) (startedChild, error) {
	support := sandbox.Support()
	target, err := resolveSpawnTarget(req.dataRoot, req.plugin, req.entryPath, req.instanceDataDir)
	if err != nil {
		return fallBackOrRefuse(req, err)
	}
	started, err := startExecChild(target, req, support.Mode())
	// The shim caps the plugin's resources itself, immediately before it becomes the plugin. See
	// the note there: applied from outside, the limit lands on the shim instead.
	started.limitsApplied = support.Available
	return started, err
}
