package wails

import (
	domainplugin "ssh-client/internal/domain/plugin"
)

// AppVersion is the user-facing application/product version shown in the About panel. It is the
// release version of xQuakShell as a whole, distinct from the plugin core version and the frozen
// plugin API envelope version (ADR-012). Keep it in sync with wails.json productVersion; this is
// the single Go source of truth so the UI carries no duplicated literal.
const AppVersion = "1.0.0"

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
		AppVersion:       AppVersion,
		CoreVersion:      domainplugin.HostCoreVersion,
		PluginAPIVersion: domainplugin.PluginAPIVersion,
	}
}
