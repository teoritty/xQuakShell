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

// NewPluginSessionRPCHandler creates a session RPC handler with mandatory scope enforcement.
func NewPluginSessionRPCHandler(
	sessions domainplugin.SessionInboundPort,
	embed *PluginEmbedInbound,
	channels domainplugin.ChannelInboundPort,
	discovery domainplugin.DiscoveryInboundPort,
	surfaces domainplugin.SurfaceInboundPort,
	dialogs domainplugin.DialogInboundPort,
	details domainplugin.DiscoveryDetailsInboundPort,
	auth domainplugin.SessionRPCAuthorizer,
	scope PluginSessionScope,
) *PluginSessionRPCHandler {
	return &PluginSessionRPCHandler{
		sessions:  sessions,
		embed:     embed,
		channels:  channels,
		discovery: discovery,
		surfaces:  surfaces,
		dialogs:   dialogs,
		details:   details,
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
	case MethodSurfaceOpen:
		// Opening a surface names the session whose authorization it borrows, so it is authorized
		// exactly where channel.open and discovery.publish are. The later surface verbs name a
		// surfaceId instead and are authorized by ownership inside SurfaceService — one rule each,
		// in one place each.
		if h.surfaces == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req surfaceOpenAuthParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.ParentSessionID); err != nil {
			return nil, err
		}
		return h.surfaces.Handle(ctx, pluginID, method, params)
	case MethodSurfaceWrite, MethodSurfaceUpdateState, MethodSurfaceSetTitle, MethodSurfaceClose:
		if h.surfaces == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return h.surfaces.Handle(ctx, pluginID, method, params)
	case MethodDiscoveryPublishDetails:
		// It names a sessionId, so it is authorized here exactly like discovery.publish.
		if h.details == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		var req discoveryPublishAuthParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.SessionID); err != nil {
			return nil, err
		}
		return h.details.PublishDetails(ctx, pluginID, params)
	case MethodDialogOpen, MethodDialogSetError, MethodDialogClose:
		// No session is named anywhere in the dialog verbs, so there is no IDOR check to run here:
		// ownership is by dialogId, decided inside DialogService, and the grant is the gate's.
		if h.dialogs == nil {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return h.dialogs.Handle(ctx, pluginID, method, params)
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

// surfaceOpenAuthParams peels off just the field authorization needs, like the two above. The rest
// of the open request is decoded once, by the surface usecase that will act on it.
type surfaceOpenAuthParams struct {
	ParentSessionID string `json:"parentSessionId"`
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
// since each plugin process gets its own ChannelProxy (ADR-011 Stage 4b) rather than a shared one.
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
		return NewPluginSessionRPCHandler(inbound, embed, channels, discovery, surfaces, dialogs, details, auth, PluginSessionScope{
			PluginID:          plugin.Manifest.ID,
			ProcessSessionID:  processSessionID,
			Isolation:         plugin.Manifest.EffectiveIsolation(),
			AllowMultiSession: allowMulti,
		})
	}
}
