//go:build !windows

package plugin

import domainplugin "xquakshell/internal/domain/plugin"

// PluginInstanceEffectiveTempDir is where a plugin process's temporary files actually land, which
// off Windows is simply the directory the host named: nothing rewrites TMPDIR behind our back.
func PluginInstanceEffectiveTempDir(_ domainplugin.InstalledPlugin, _ string, instanceDataDir string) string {
	return PluginInstanceTempDir(instanceDataDir)
}
