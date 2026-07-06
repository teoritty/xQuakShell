package plugin

import (
	"context"
	"encoding/json"
	"time"
)

// CallWithTimeout invokes a JSON-RPC method with an explicit timeout override.
func (h *ProcessHost) CallWithTimeout(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	mp, err := h.runningProcess(pluginID, sessionID)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return mp.conn.Call(callCtx, method, params)
}

// Call invokes a JSON-RPC method using the default call timeout.
func (h *ProcessHost) Call(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	return h.CallWithTimeout(ctx, pluginID, sessionID, method, params, callTimeout)
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
