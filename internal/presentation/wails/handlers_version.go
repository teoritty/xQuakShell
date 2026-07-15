package wails

import (
	"ssh-client/internal/appinfo"
	domainplugin "ssh-client/internal/domain/plugin"
)

// VersionInfoDTO carries the three distinct versions surfaced in the About panel: the application
// release, the plugin core (backend engine) version, and the frozen plugin API envelope version
// (ADR-012). All three are sourced from single Go constants — no duplicated literals in the UI.
type VersionInfoDTO struct {
	AppVersion       string `json:"appVersion"`
	CoreVersion      string `json:"coreVersion"`
	PluginAPIVersion string `json:"pluginApiVersion"`
}

// GetVersionInfo returns the application, core, and plugin API versions for display.
func (a *AppAPI) GetVersionInfo() VersionInfoDTO {
	return VersionInfoDTO{
		AppVersion:       appinfo.AppVersion,
		CoreVersion:      domainplugin.HostCoreVersion,
		PluginAPIVersion: domainplugin.PluginAPIVersion,
	}
}
