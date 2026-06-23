package plugin

import (
	"context"
	"encoding/json"
)

// ViewInboundPort handles plugin→UI view messages (Phase 5).
type ViewInboundPort interface {
	PostMessage(_ context.Context, pluginID, panelID string, message json.RawMessage) error
}
