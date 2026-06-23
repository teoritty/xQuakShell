package plugin

import (
	"context"
	"encoding/json"
)

// EventInboundPort handles plugin event bus RPC (subscribe/publish).
type EventInboundPort interface {
	Subscribe(ctx context.Context, pluginID, channel string) error
	PublishFromPlugin(ctx context.Context, pluginID, channel string, payload json.RawMessage) error
}
