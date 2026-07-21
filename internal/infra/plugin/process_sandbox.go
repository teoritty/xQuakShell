package plugin

import (
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

func preparePluginSandbox(dataRoot string, plugin domainplugin.InstalledPlugin, sessionID string, pid int) (pluginJob, string, error) {
	job, err := createPluginJob()
	if err != nil {
		return pluginJob{}, "", fmt.Errorf("create plugin job: %w", err)
	}
	if err := assignProcessToJob(job, pid); err != nil {
		closePluginJob(job)
		return pluginJob{}, "", fmt.Errorf("assign plugin %s to job: %w", plugin.Manifest.ID, err)
	}
	if err := applyPluginResourceLimits(pid, job); err != nil {
		closePluginJob(job)
		return pluginJob{}, "", fmt.Errorf("apply plugin %s resource limits: %w", plugin.Manifest.ID, err)
	}

	isolation := plugin.Manifest.EffectiveIsolation()
	dataDir, err := EnsurePluginInstanceDataDir(dataRoot, plugin.Manifest.ID, sessionID, isolation)
	if err != nil {
		closePluginJob(job)
		return pluginJob{}, "", fmt.Errorf("create plugin data dir: %w", err)
	}
	return job, dataDir, nil
}
