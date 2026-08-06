package plugin

import (
	"context"
	"encoding/json"
)

// DialogInboundPort handles plugin->host dialog.* RPC, mirroring SurfaceInboundPort.
type DialogInboundPort interface {
	Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)
}

// DialogOutboundPort delivers a dialog's answer to its owner.
//
// Exactly one of these arrives for any dialog the host opened — including when the host closes it
// during teardown, which is a cancellation. That guarantee is what lets a plugin await an answer
// without a timeout of its own.
type DialogOutboundPort interface {
	Submitted(pluginID, dialogID string, values map[string]string)
	Cancelled(pluginID, dialogID string)
}

// DialogPluginCloser cancels every dialog a plugin owns. Called when its process stops: a modal
// whose owner is gone can never be answered.
type DialogPluginCloser interface {
	CancelForPlugin(pluginID string)
}
