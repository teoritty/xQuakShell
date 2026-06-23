package wails

import domainplugin "ssh-client/internal/domain/plugin"

// ConnectionProtocolDTO is a plugin-contributed connection protocol.
type ConnectionProtocolDTO struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	DefaultPort int    `json:"defaultPort,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// GetPluginConnectionProtocols returns merged protocol contributions for the UI.
func (a *AppAPI) GetPluginConnectionProtocols() []ConnectionProtocolDTO {
	if a.plugins == nil {
		return []ConnectionProtocolDTO{}
	}
	protocols := a.plugins.Registry().ConnectionProtocols()
	out := make([]ConnectionProtocolDTO, 0, len(protocols)+1)
	out = append(out, ConnectionProtocolDTO{
		ID:          "ssh",
		Label:       "SSH",
		DefaultPort: 22,
		Icon:        "terminal",
	})
	for _, p := range protocols {
		out = append(out, ConnectionProtocolDTO{
			ID:          p.ID,
			Label:       p.Label,
			DefaultPort: p.DefaultPort,
			Icon:        p.Icon,
		})
	}
	return out
}

func mapConnectionProtocol(p domainplugin.ConnectionProtocolContribution) ConnectionProtocolDTO {
	return ConnectionProtocolDTO{
		ID:          p.ID,
		Label:       p.Label,
		DefaultPort: p.DefaultPort,
		Icon:        p.Icon,
	}
}
