package plugin

import (
	"context"
	"encoding/json"
)

// SessionRPCHandler dispatches plugin session.* RPC methods after usecase authorization.
type SessionRPCHandler interface {
	Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)
}

// SessionRPCHandlerFactory builds a session RPC handler for a plugin process instance. channels
// is that specific process's own ChannelInboundPort (one ChannelProxy per managedProcess,
// ADR-011) — it must not be a shared/global port, since channelId allocation and
// ownership are scoped to a single plugin process.
type SessionRPCHandlerFactory func(plugin InstalledPlugin, processSessionID string, channels ChannelInboundPort) SessionRPCHandler
