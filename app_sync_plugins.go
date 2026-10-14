package main

import "xquakshell/internal/presentation/wails"

// Wails facade for plugin sync (ADR-010, spec §6.4-6.5): one-line delegates, no logic.

// ListSyncPluginOffers lists the plugins from the user's other devices that need them here.
func (a *App) ListSyncPluginOffers() ([]wails.SyncPluginOfferDTO, error) {
	return a.api.ListSyncPluginOffers()
}

// PreviewSyncPluginOffer prepares the consent screen for an offered plugin.
func (a *App) PreviewSyncPluginOffer(pluginID string) (wails.GitHubPluginPreviewResponseDTO, error) {
	return a.api.PreviewSyncPluginOffer(pluginID)
}

// InstallSyncPluginOffer installs an offered plugin, held to the build the other device recorded.
func (a *App) InstallSyncPluginOffer(req wails.SyncPluginInstallRequest) (wails.SyncPluginInstallResultDTO, error) {
	return a.api.InstallSyncPluginOffer(req)
}

// KeepSyncPlugin keeps the build installed here as the version every device is offered.
func (a *App) KeepSyncPlugin(pluginID string) error { return a.api.KeepSyncPlugin(pluginID) }

// ListSyncPluginData lists the plugins whose data follows the user.
func (a *App) ListSyncPluginData() ([]wails.SyncPluginDataDTO, error) {
	return a.api.ListSyncPluginData()
}

// SetSyncPluginData turns syncing of one plugin's data on or off on this device only.
func (a *App) SetSyncPluginData(pluginID string, enabled bool) error {
	return a.api.SetSyncPluginData(pluginID, enabled)
}

// RestartSyncPlugin restarts a plugin so the data updates waiting for it are applied.
func (a *App) RestartSyncPlugin(pluginID string) error { return a.api.RestartSyncPlugin(pluginID) }
