package plugin

import (
	"encoding/json"
)

// Tunnel RPC redaction.
//
// Nothing in internal/ calls these two functions today: tunnel.* traffic is not
// routed through the outbound audit hook the way auth.* is (see
// usecase.PluginManager). They are kept rather than deleted because a redactor
// is the thing you want already written and already tested at the moment you
// decide to start auditing a channel that carries relayed bytes — reconstructing
// one under that pressure is how the enumerated-shape bug this file used to
// share got written in the first place. Sharing sanitizeRPC keeps it honest at
// no cost: it cannot drift from the auth redactor, because there is only one.

// sensitiveTunnelKeys is deliberately narrower than the auth set. A tunnel frame
// carries relayed application bytes, not credentials; the surrounding
// connection ids stay readable so an audit entry can still be traced.
var sensitiveTunnelKeys = map[string]struct{}{
	"dataBase64": {},
}

// SanitizeTunnelRPCParams redacts sensitive tunnel.* RPC payload fields before audit/logging.
func SanitizeTunnelRPCParams(method string, params json.RawMessage) json.RawMessage {
	return sanitizeRPC("tunnel.", method, params, sensitiveTunnelKeys)
}

// SanitizeTunnelRPCResult redacts sensitive tunnel.* RPC result fields before audit/logging.
func SanitizeTunnelRPCResult(method string, result json.RawMessage) json.RawMessage {
	return sanitizeRPC("tunnel.", method, result, sensitiveTunnelKeys)
}
