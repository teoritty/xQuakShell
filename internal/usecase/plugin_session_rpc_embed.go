package usecase

import (
	"context"
	"encoding/json"

	domainplugin "xquakshell/internal/domain/plugin"
)

// The embed and tunnel half of the session RPC dispatcher (ADR-011).
//
// Five verbs with one rule between them: each names a session, none may touch a session the plugin
// does not hold, and each then forwards to the embed service. The rule is written once here rather
// than five times, which is also what makes it impossible to add a sixth verb that forgets it.
func (h *PluginSessionRPCHandler) handleEmbedVerb(
	ctx context.Context,
	pluginID, method string,
	params json.RawMessage,
) (json.RawMessage, error) {
	if h.embed == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	// Every verb here names its session in the same field, so authorization reads it once and the
	// specific payload is decoded afterwards by whichever case needs it.
	var named struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &named); err != nil {
		return nil, err
	}
	if err := h.authorize(named.SessionID); err != nil {
		return nil, err
	}

	switch method {
	case "session.registerEmbed":
		var req registerEmbedParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return h.embed.RegisterEmbed(ctx, pluginID, req.SessionID, req.UIEntry, NormalizeTunnelIDs(req.TunnelIDs))
	case "session.reportLocalEmbed":
		if err := h.embed.ReportLocalEmbed(ctx, pluginID, named.SessionID, params); err != nil {
			return nil, err
		}
	default:
		if err := h.handleTunnelVerb(ctx, pluginID, method, params); err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// handleTunnelVerb dispatches the three tunnel verbs, which share a payload as well as a rule.
func (h *PluginSessionRPCHandler) handleTunnelVerb(
	ctx context.Context,
	pluginID, method string,
	params json.RawMessage,
) error {
	var req tunnelParams
	if err := json.Unmarshal(params, &req); err != nil {
		return err
	}
	switch method {
	case "session.tunnelOpen":
		return h.embed.TunnelOpen(ctx, pluginID, req.SessionID, req.TunnelID)
	case "session.tunnelFrame":
		return h.embed.TunnelFrame(ctx, pluginID, req.SessionID, req.TunnelID, req.DataBase64, req.EOF)
	case "session.tunnelClose":
		return h.embed.TunnelClose(ctx, pluginID, req.SessionID, req.TunnelID)
	default:
		return domainplugin.ErrCapabilityDenied
	}
}
