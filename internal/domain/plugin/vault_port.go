package plugin

import (
	"context"
	"encoding/json"
)

// VaultInboundPort handles plugin→core vault RPC with usecase-level authz.
type VaultInboundPort interface {
	GetConnection(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	GetSecret(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
}
