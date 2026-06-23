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
	scope    PluginSessionScope
	auth     domainplugin.SessionRPCAuthorizer
}

// NewPluginSessionRPCHandler creates a session RPC handler with mandatory scope enforcement.
func NewPluginSessionRPCHandler(
	sessions domainplugin.SessionInboundPort,
	auth domainplugin.SessionRPCAuthorizer,
	scope PluginSessionScope,
) *PluginSessionRPCHandler {
	return &PluginSessionRPCHandler{
		sessions: sessions,
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
	auth domainplugin.SessionRPCAuthorizer,
) domainplugin.SessionRPCHandlerFactory {
	return func(plugin domainplugin.InstalledPlugin, processSessionID string) domainplugin.SessionRPCHandler {
		allowMulti := false
		if plugin.Manifest.Capabilities.Session != nil {
			allowMulti = plugin.Manifest.Capabilities.Session.AllowMultiSession
		}
		return NewPluginSessionRPCHandler(inbound, auth, PluginSessionScope{
			PluginID:          plugin.Manifest.ID,
			ProcessSessionID:  processSessionID,
			Isolation:         plugin.Manifest.EffectiveIsolation(),
			AllowMultiSession: allowMulti,
		})
	}
}
