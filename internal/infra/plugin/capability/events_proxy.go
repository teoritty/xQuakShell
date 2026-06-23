package capability

import (
	"context"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// EventsProxy forwards plugin event bus RPC to the core event inbound port.
type EventsProxy struct {
	events domainplugin.EventInboundPort
}

// NewEventsProxy creates an events RPC proxy.
func NewEventsProxy(events domainplugin.EventInboundPort) *EventsProxy {
	return &EventsProxy{events: events}
}

type eventChannelParams struct {
	Channel string `json:"channel"`
}

type eventPublishParams struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}

// Handle dispatches events.* plugin RPC methods.
func (p *EventsProxy) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	if p.events == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	switch method {
	case "events.subscribe":
		var req eventChannelParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := p.events.Subscribe(ctx, pluginID, req.Channel); err != nil {
			return nil, err
		}
	case "events.publish":
		var req eventPublishParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := p.events.PublishFromPlugin(ctx, pluginID, req.Channel, req.Payload); err != nil {
			return nil, err
		}
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
	return json.Marshal(map[string]bool{"ok": true})
}
