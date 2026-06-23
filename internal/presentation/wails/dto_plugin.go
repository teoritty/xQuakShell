package wails

import "ssh-client/internal/usecase"

// PluginDTO describes an installed plugin for the frontend.
type PluginDTO struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Description          string `json:"description"`
	Source               string `json:"source"`
	State                string `json:"state"`
	RequiresSecretAccess bool   `json:"requiresSecretAccess"`
	Signed               bool   `json:"signed"`
	Enabled              bool   `json:"enabled"`
}

// PluginPingResultDTO is returned by PingPlugin.
type PluginPingResultDTO struct {
	PluginID string            `json:"pluginId"`
	Result   map[string]string `json:"result"`
}

func pluginInfoToDTO(info usecase.PluginInfo) PluginDTO {
	return PluginDTO{
		ID:                   info.ID,
		Name:                 info.Name,
		Version:              info.Version,
		Description:          info.Description,
		Source:               info.Source,
		State:                info.State,
		RequiresSecretAccess: info.RequiresSecretAccess,
		Signed:               info.Signed,
		Enabled:              info.Enabled,
	}
}
