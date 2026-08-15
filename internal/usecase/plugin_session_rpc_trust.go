package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"

	domainplugin "xquakshell/internal/domain/plugin"
)

// The half of the dispatcher that answers questions about the remote peer's identity.
//
// Lifted out of Handle for the same reason the embed verbs next door are: a function size budget
// is on duty there, and one verb carrying parsing, authorization and decoding goes over it. But
// the budget is not the only reason - a trust boundary lives here, and reading it in the middle of
// a flat switch is how you miss the order the checks come in.

// verifyPeerParams are the parameters of trust.verifyPeer.
type verifyPeerParams struct {
	SessionID string `json:"sessionId"`
	// Subject is the plugin's belief about who it is talking to. It is checked against the
	// connection record and not used afterwards: both the dialog and the storage get the core's
	// version. The field exists so a disagreement becomes visible.
	Subject string `json:"subject"`
	// MaterialBase64 is the bytes observed on the other side. The core computes the fingerprint
	// itself: accepting a ready-made one would allow showing one value and storing another.
	MaterialBase64 string `json:"materialBase64"`
}

// handleVerifyPeer serves trust.verifyPeer.
//
// The order of the steps is load-bearing: session ownership first, decoding second, and only then
// the trust question. A plugin that does not own the session must not reach even the creation of a
// dialog - otherwise the user would be shown a question about a connection they never opened.
//
// The scope is h.scope.PluginID, the same value authorize() checks against, not the pluginID the
// dispatcher was handed. One quantity behind both the authorization and the write: two sources
// drift, and in a trust boundary the drift is the vulnerability.
func (h *PluginSessionRPCHandler) handleVerifyPeer(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if h.peerTrust == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	var req verifyPeerParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	// Session ownership is checked here and only here - as with channel.open and
	// discovery.publish. Two authorizations of one rule drift apart, and the weaker one becomes
	// the way in.
	if err := h.authorize(req.SessionID); err != nil {
		return nil, err
	}
	material, err := base64.StdEncoding.DecodeString(req.MaterialBase64)
	if err != nil {
		return nil, err
	}
	trusted, err := h.peerTrust.VerifyPeer(ctx, h.scope.PluginID, req.SessionID, req.Subject, material)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"trusted": trusted})
}
