package plugin

import (
	"context"
	"encoding/json"
)

// SessionRPCHandler dispatches plugin session.* RPC methods after usecase authorization.
type SessionRPCHandler interface {
	Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)
}

// SessionRPCHandlerFactory builds a session RPC handler for a plugin process instance.
type SessionRPCHandlerFactory func(plugin InstalledPlugin, processSessionID string) SessionRPCHandler
