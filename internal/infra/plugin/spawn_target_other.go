//go:build !linux

package plugin

import domainplugin "xquakshell/internal/domain/plugin"

// resolveSpawnTarget execs the plugin binary directly, because no other platform can confine it
// yet. Windows will not reach it through here at all: an AppContainer child cannot be an *exec.Cmd,
// so it arrives behind the childProcess seam rather than as a different argv.
func resolveSpawnTarget(_ string, _ domainplugin.InstalledPlugin, entryPath, _ string) (spawnTarget, error) {
	return spawnTarget{path: entryPath}, nil
}
