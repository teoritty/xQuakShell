package usecase

import (
	"context"
	"encoding/json"

	domainplugin "xquakshell/internal/domain/plugin"
)

// The ADR-015 half of the session RPC dispatcher: surfaces, dialogs and node details.
//
// Split from plugin_session_rpc.go by verb family rather than by anything else, because the
// families differ in exactly one interesting way — what each has to authorize — and putting that
// difference on its own makes it readable instead of inferable from a long switch.

// handleSurfaceVerb dispatches surface.*.
//
// Only open names a session, and it is authorized here, on the same path and by the same
// authorizer that guards channel.open and discovery.publish. The later verbs name a surfaceId
// instead and are authorized by ownership inside SurfaceService — one rule each, in one place each.
func (h *PluginSessionRPCHandler) handleSurfaceVerb(
	ctx context.Context,
	pluginID, method string,
	params json.RawMessage,
) (json.RawMessage, error) {
	if h.surfaces == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if method == MethodSurfaceOpen {
		var req surfaceOpenAuthParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := h.authorize(req.ParentSessionID); err != nil {
			return nil, err
		}
	}
	return h.surfaces.Handle(ctx, pluginID, method, params)
}

// handleDialogVerb dispatches dialog.*.
//
// No session is named anywhere in the dialog verbs, so there is no IDOR check to run: ownership is
// by dialogId, decided inside DialogService, and the grant is the gate's.
func (h *PluginSessionRPCHandler) handleDialogVerb(
	ctx context.Context,
	pluginID, method string,
	params json.RawMessage,
) (json.RawMessage, error) {
	if h.dialogs == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return h.dialogs.Handle(ctx, pluginID, method, params)
}

// handlePublishDetails dispatches discovery.publishDetails, which names a sessionId and is
// therefore authorized exactly like discovery.publish.
func (h *PluginSessionRPCHandler) handlePublishDetails(
	ctx context.Context,
	pluginID string,
	params json.RawMessage,
) (json.RawMessage, error) {
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
}

// surfaceOpenAuthParams peels off just the field authorization needs, like its channel and
// discovery counterparts. The rest of the open request is decoded once, by the surface usecase
// that will act on it.
type surfaceOpenAuthParams struct {
	ParentSessionID string `json:"parentSessionId"`
}
