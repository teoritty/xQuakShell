//go:build !windows

package plugin

// startPluginChild spawns through os/exec, which is the whole story everywhere but Windows.
//
// On Linux the confinement is already in the target: resolveSpawnTarget names the sandbox shim and
// puts the plugin binary in its arguments, so the process created here narrows itself and then
// becomes the plugin. Nothing about how it is created has to change, which is exactly why the
// Landlock design was worth preferring.
func startPluginChild(req childRequest) (startedChild, error) {
	target, err := resolveSpawnTarget(req.dataRoot, req.plugin, req.entryPath, req.instanceDataDir)
	if err != nil {
		return startedChild{}, err
	}
	return startExecChild(target, req)
}
