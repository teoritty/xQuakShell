package plugin

import "context"

// SessionInboundPort handles plugin→core session RPC callbacks.
type SessionInboundPort interface {
	UpdateState(ctx context.Context, pluginID, sessionID, state, errMsg string) error
	WriteTerminal(ctx context.Context, pluginID, sessionID, dataBase64 string) error
}
