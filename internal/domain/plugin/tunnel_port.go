package plugin

import (
	"context"
	"encoding/json"
)

// TunnelInboundPort handles plugin→core dynamic-forward tunnel RPC with usecase-level routing.
type TunnelInboundPort interface {
	TunnelDial(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	TunnelClose(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	TunnelLocalWrite(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	TunnelLocalClose(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	TunnelBind(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
}
