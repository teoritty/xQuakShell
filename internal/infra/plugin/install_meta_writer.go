package plugin

import domainplugin "ssh-client/internal/domain/plugin"

// InstallMetaWriter adapts WriteInstallMeta to the usecase GitHubInstallMetaWriter port.
type InstallMetaWriter struct{}

// Write persists install provenance into a staged plugin directory.
func (InstallMetaWriter) Write(stageDir string, meta domainplugin.PluginInstallMeta) error {
	return WriteInstallMeta(stageDir, meta)
}
