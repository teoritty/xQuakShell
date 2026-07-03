package usecase

import (
	"context"
	"encoding/json"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginSessionScope binds a plugin process instance for session RPC authorization.
type PluginSessionScope struct {
	PluginID          string
	ProcessSessionID  string
	Isolation         domainplugin.IsolationMode
	AllowMultiSession bool
}

// PluginSessionRPCHandler enforces session scope in the usecase layer before forwarding RPC.
type PluginSessionRPCHandler struct {
	sessions domainplugin.SessionInboundPort
	embed    *PluginEmbedInbound
	scope    PluginSessionScope
	auth     domainplugin.SessionRPCAuthorizer
}

// NewPluginSessionRPCHandler creates a session RPC handler with mandatory scope enforcement.
func NewPluginSessionRPCHandler(
	sessions domainplugin.SessionInboundPort,
	embed *PluginEmbedInbound,
	auth domainplugin.SessionRPCAuthorizer,
	scope PluginSessionScope,
) *PluginSessionRPCHandler {
	return &PluginSessionRPCHandler{
		sessions: sessions,
		embed:    embed,
		scope:    scope,
		auth:     auth,
	}
}

type sessionUpdateParams struct {
	SessionID string `json:"sessionId"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

type sessionTerminalParams struct {
	SessionID    string `json:"sessionId"`
	OutputBase64 string `json:"outputBase64"`
}

type registerEmbedParams struct {
	SessionID string   `json:"sessionId"`
	UIEntry   string   `json:"uiEntry"`
	TunnelIDs []string `json:"tunnelIds"`
}

type tunnelParams struct {
	SessionID    string `json:"sessionId"`
	TunnelID     string `json:"tunnelId"`
	DataBase64   string `json:"dataBase64,omitempty"`
	EOF          bool   `json:"eof,omitempty"`
}

// Handle dispatches session.* plugin RPC methods.
func (h *PluginSessionRPCHandler) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	if h.sessions == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	switch method {
	case "session.updateState":
		var req sessionUpdateParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.sessions.UpdateState(ctx, pluginID, req.SessionID, req.State, req.Error); err != nil {
			return nil, err
		}
	case "session.writeTerminal":
		var req sessionTerminalParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.sessions.WriteTerminal(ctx, pluginID, req.SessionID, req.OutputBase64); err != nil {
			return nil, err
		}
	case "session.registerEmbed":
		if h.embed == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req registerEmbedParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		return h.embed.RegisterEmbed(ctx, pluginID, req.SessionID, req.UIEntry, NormalizeTunnelIDs(req.TunnelIDs))
	case "session.tunnelOpen":
		if h.embed == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req tunnelParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.embed.TunnelOpen(ctx, pluginID, req.SessionID, req.TunnelID); err != nil {
			return nil, err
		}
	case "session.tunnelFrame":
		if h.embed == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req tunnelParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.embed.TunnelFrame(ctx, pluginID, req.SessionID, req.TunnelID, req.DataBase64, req.EOF); err != nil {
			return nil, err
		}
	case "session.tunnelClose":
		if h.embed == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req tunnelParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.embed.TunnelClose(ctx, pluginID, req.SessionID, req.TunnelID); err != nil {
			return nil, err
		}
	case "session.reportLocalEmbed":
		if h.embed == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if err := h.embed.ReportLocalEmbed(ctx, pluginID, req.SessionID, params); err != nil {
			return nil, err
		}
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
	return json.Marshal(map[string]bool{"ok": true})
}

func (h *PluginSessionRPCHandler) authorize(targetSessionID string) error {
	targetSessionID = strings.TrimSpace(targetSessionID)
	if targetSessionID == "" {
		return domainplugin.ErrSessionNotBound
	}
	if h.auth == nil {
		return domainplugin.ErrSessionNotBound
	}
	return h.auth.AuthorizeSessionRPC(
		h.scope.PluginID,
		h.scope.ProcessSessionID,
		h.scope.Isolation,
		h.scope.AllowMultiSession,
		targetSessionID,
	)
}

var _ domainplugin.SessionRPCHandler = (*PluginSessionRPCHandler)(nil)

// NewPluginSessionRPCHandlerFactory returns a factory wired to inbound session RPC and authorizer.
func NewPluginSessionRPCHandlerFactory(
	inbound domainplugin.SessionInboundPort,
	embed *PluginEmbedInbound,
	auth domainplugin.SessionRPCAuthorizer,
) domainplugin.SessionRPCHandlerFactory {
	return func(plugin domainplugin.InstalledPlugin, processSessionID string) domainplugin.SessionRPCHandler {
		allowMulti := false
		if plugin.Manifest.Capabilities.Session != nil {
			allowMulti = plugin.Manifest.Capabilities.Session.AllowMultiSession
		}
		return NewPluginSessionRPCHandler(inbound, embed, auth, PluginSessionScope{
			PluginID:          plugin.Manifest.ID,
			ProcessSessionID:  processSessionID,
			Isolation:         plugin.Manifest.EffectiveIsolation(),
			AllowMultiSession: allowMulti,
		})
	}
}
