package plugin

import (
	"context"
	"encoding/json"
)

// Call invokes a JSON-RPC method on a running plugin.
func (h *ProcessHost) Call(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	mp, err := h.runningProcess(pluginID, sessionID)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return mp.conn.Call(callCtx, method, params)
}

// Notify sends a JSON-RPC notification to a running plugin.
func (h *ProcessHost) Notify(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
	mp, err := h.runningProcess(pluginID, sessionID)
	if err != nil {
		return err
	}
	_ = ctx
	return mp.conn.Notify(method, params)
}
