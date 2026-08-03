package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type tunnelLocalNotifyParams struct {
	LocalConnID string `json:"localConnId"`
}

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
	h.syncTunnelLocalNotify(mp, method, params)
	_ = ctx
	return mp.conn.Notify(method, params)
}

func (h *ProcessHost) syncTunnelLocalNotify(mp *managedProcess, method string, params json.RawMessage) {
	if mp == nil || mp.tunnelLocal == nil {
		return
	}
	switch method {
	case "tunnel.localAccept":
		if localConnID := parseTunnelLocalConnID(params); localConnID != "" {
			mp.tunnelLocal.RegisterLocal(localConnID)
		}
	case "tunnel.localClose":
		if localConnID := parseTunnelLocalConnID(params); localConnID != "" {
			mp.tunnelLocal.ReleaseLocal(localConnID)
		}
	}
}

func parseTunnelLocalConnID(params json.RawMessage) string {
	var req tunnelLocalNotifyParams
	if err := json.Unmarshal(params, &req); err != nil {
		return ""
	}
	return strings.TrimSpace(req.LocalConnID)
}
