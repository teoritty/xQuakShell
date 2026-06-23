package capability

import (
	"context"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ViewProxy forwards plugin view RPC to the core view inbound port.
type ViewProxy struct {
	views domainplugin.ViewInboundPort
}

// NewViewProxy creates a view RPC proxy.
func NewViewProxy(views domainplugin.ViewInboundPort) *ViewProxy {
	return &ViewProxy{views: views}
}

type viewMessageParams struct {
	PanelID string          `json:"panelId"`
	Message json.RawMessage `json:"message"`
}

// Handle dispatches view.* plugin RPC methods.
func (p *ViewProxy) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	if p.views == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	switch method {
	case "view.postMessage":
		var req viewMessageParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := p.views.PostMessage(ctx, pluginID, req.PanelID, req.Message); err != nil {
			return nil, err
		}
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
	return json.Marshal(map[string]bool{"ok": true})
}
