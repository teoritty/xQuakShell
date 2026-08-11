package plugin

import (
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// preparePluginInstanceDirs creates everything the plugin process is allowed to write, and must run
// before the process is spawned: the temp directory's path goes into the child's environment, so it
// cannot be created after the child that is already using it.
func preparePluginInstanceDirs(dataRoot string, plugin domainplugin.InstalledPlugin, sessionID string) (string, error) {
	isolation := plugin.Manifest.EffectiveIsolation()
	dataDir, err := EnsurePluginInstanceDataDir(dataRoot, plugin.Manifest.ID, sessionID, isolation)
	if err != nil {
		return "", fmt.Errorf("create plugin %s data dir: %w", plugin.Manifest.ID, err)
	}
	if _, err := EnsurePluginInstanceTempDir(dataDir); err != nil {
		return "", fmt.Errorf("create plugin %s temp dir: %w", plugin.Manifest.ID, err)
	}
	return dataDir, nil
}

// preparePluginSandbox applies the OS-level bounds on the spawned process. It needs a pid, so it
// necessarily runs after the spawn — which is why the directories are a separate step above.
func preparePluginSandbox(plugin domainplugin.InstalledPlugin, pid int) (pluginJob, error) {
	job, err := createPluginJob()
	if err != nil {
		return pluginJob{}, fmt.Errorf("create plugin job: %w", err)
	}
	if err := assignProcessToJob(job, pid); err != nil {
		closePluginJob(job)
		return pluginJob{}, fmt.Errorf("assign plugin %s to job: %w", plugin.Manifest.ID, err)
	}
	if err := applyPluginResourceLimits(pid, job); err != nil {
		closePluginJob(job)
		return pluginJob{}, fmt.Errorf("apply plugin %s resource limits: %w", plugin.Manifest.ID, err)
	}
	return job, nil
}
