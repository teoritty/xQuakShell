//go:build windows

package plugin

import (
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// PluginInstanceEffectiveTempDir is where a plugin process's temporary files actually land.
//
// It is not always PluginInstanceTempDir, and that is a fact about AppContainers rather than a
// choice made here: Windows computes a container's TEMP and TMP from its LOCALAPPDATA and
// overwrites whatever the parent set, so a confined plugin never sees the values PluginProcessEnv
// put in its environment. Pointing LOCALAPPDATA at the instance directory is what keeps the result
// inside the one root the plugin is granted — see sandbox.ContainerTempDir — but the path is a
// level down from the one an unconfined plugin gets, and anything that has to find those files
// must ask rather than assume.
func PluginInstanceEffectiveTempDir(plugin domainplugin.InstalledPlugin, sessionID, instanceDataDir string) string {
	if !sandbox.Support().Available {
		return PluginInstanceTempDir(instanceDataDir)
	}
	return sandbox.ContainerTempDir(instanceDataDir, pluginContainerName(plugin, sessionID))
}
