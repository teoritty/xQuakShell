package usecase

import (
	"context"
	"encoding/json"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
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
	sessions  domainplugin.SessionInboundPort
	embed     *PluginEmbedInbound
	channels  domainplugin.ChannelInboundPort
	discovery domainplugin.DiscoveryInboundPort
	surfaces  domainplugin.SurfaceInboundPort
	dialogs   domainplugin.DialogInboundPort
	details   domainplugin.DiscoveryDetailsInboundPort
	scope     PluginSessionScope
	auth      domainplugin.SessionRPCAuthorizer
}

// PluginSessionRPCPorts is every inbound port this handler can dispatch to.
//
// A struct rather than a parameter list: seven of these are interfaces, several have the same
// shape, and two transposed arguments would have compiled and quietly routed one verb family into
// another's service. Named fields make that mistake impossible to write.
type PluginSessionRPCPorts struct {
	Sessions  domainplugin.SessionInboundPort
	Embed     *PluginEmbedInbound
	Channels  domainplugin.ChannelInboundPort
	Discovery domainplugin.DiscoveryInboundPort
	Surfaces  domainplugin.SurfaceInboundPort
	Dialogs   domainplugin.DialogInboundPort
	Details   domainplugin.DiscoveryDetailsInboundPort
}

// NewPluginSessionRPCHandler creates a session RPC handler with mandatory scope enforcement.
func NewPluginSessionRPCHandler(
	ports PluginSessionRPCPorts,
	auth domainplugin.SessionRPCAuthorizer,
	scope PluginSessionScope,
) *PluginSessionRPCHandler {
	return &PluginSessionRPCHandler{
		sessions:  ports.Sessions,
		embed:     ports.Embed,
		channels:  ports.Channels,
		discovery: ports.Discovery,
		surfaces:  ports.Surfaces,
		dialogs:   ports.Dialogs,
		details:   ports.Details,
		scope:     scope,
		auth:      auth,
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
	SessionID  string `json:"sessionId"`
	TunnelID   string `json:"tunnelId"`
	DataBase64 string `json:"dataBase64,omitempty"`
	EOF        bool   `json:"eof,omitempty"`
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
	// The embed and tunnel verbs dispatch next door, in plugin_session_rpc_embed.go. They are five
	// variations on one rule — name a session, prove it is yours, forward — and reading them here
	// buried the three families that differ.
	case "session.registerEmbed", "session.tunnelOpen", "session.tunnelFrame", "session.tunnelClose", "session.reportLocalEmbed":
		return h.handleEmbedVerb(ctx, pluginID, method, params)
	case "channel.open":
		if h.channels == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req channelOpenAuthParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.ParentSessionID); err != nil {
			return nil, err
		}
		return h.channels.Open(ctx, pluginID, params)
	case "channel.close":
		if h.channels == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return h.channels.Close(ctx, pluginID, params)
	case "discovery.publish":
		// A publish names the session it enumerated through, so it is an IDOR target exactly like
		// channel.open, and it is authorized on exactly the same path: this handler, this
		// authorizer, before the payload reaches anything that could act on it. The discovery
		// usecase deliberately does NOT repeat the check (see DiscoveryPublishRouter.Apply) —
		// two authorizations of one rule drift apart, and the weaker one becomes the way in.
		if h.discovery == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req discoveryPublishAuthParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		if _, err := h.discovery.Publish(ctx, pluginID, params); err != nil {
			return nil, err
		}
	// The ADR-015 families dispatch next door, in plugin_session_rpc_ui.go: each authorizes a
	// different thing, and that difference is the only interesting part of them.
	case MethodSurfaceOpen, MethodSurfaceWrite, MethodSurfaceUpdateState, MethodSurfaceSetTitle, MethodSurfaceClose:
		return h.handleSurfaceVerb(ctx, pluginID, method, params)
	case MethodDialogOpen, MethodDialogSetError, MethodDialogClose:
		return h.handleDialogVerb(ctx, pluginID, method, params)
	case MethodDiscoveryPublishDetails:
		return h.handlePublishDetails(ctx, pluginID, params)
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
	return json.Marshal(map[string]bool{"ok": true})
}

type channelOpenAuthParams struct {
	ParentSessionID string `json:"parentSessionId"`
}

// discoveryPublishAuthParams peels off just the field authorization needs. The rest of the
// snapshot is decoded once, by the discovery usecase that will act on it; decoding it twice would
// give this layer an opinion about a payload shape it has no business knowing.
type discoveryPublishAuthParams struct {
	SessionID string `json:"sessionId"`
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
// channels is supplied per-call by the caller (internal/infra/plugin/process_ipc_factory.go),
// since each plugin process gets its own ChannelProxy (ADR-011) rather than a shared one.
// discovery is shared rather than per-process: unlike a channel, a discovery subtree belongs to a
// connection and outlives any one plugin process, so there is exactly one service behind it.
func NewPluginSessionRPCHandlerFactory(
	inbound domainplugin.SessionInboundPort,
	embed *PluginEmbedInbound,
	discovery domainplugin.DiscoveryInboundPort,
	surfaces domainplugin.SurfaceInboundPort,
	dialogs domainplugin.DialogInboundPort,
	details domainplugin.DiscoveryDetailsInboundPort,
	auth domainplugin.SessionRPCAuthorizer,
) domainplugin.SessionRPCHandlerFactory {
	return func(plugin domainplugin.InstalledPlugin, processSessionID string, channels domainplugin.ChannelInboundPort) domainplugin.SessionRPCHandler {
		allowMulti := false
		if plugin.Manifest.Capabilities.Session != nil {
			allowMulti = plugin.Manifest.Capabilities.Session.AllowMultiSession
		}
		return NewPluginSessionRPCHandler(PluginSessionRPCPorts{
			Sessions:  inbound,
			Embed:     embed,
			Channels:  channels,
			Discovery: discovery,
			Surfaces:  surfaces,
			Dialogs:   dialogs,
			Details:   details,
		}, auth, PluginSessionScope{
			PluginID:          plugin.Manifest.ID,
			ProcessSessionID:  processSessionID,
			Isolation:         plugin.Manifest.EffectiveIsolation(),
			AllowMultiSession: allowMulti,
		})
	}
}
