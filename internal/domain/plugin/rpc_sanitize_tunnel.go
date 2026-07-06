package plugin

import (
	"encoding/json"
)

// SanitizeTunnelRPCParams redacts sensitive tunnel.* RPC payload fields before audit/logging.
func SanitizeTunnelRPCParams(method string, params json.RawMessage) json.RawMessage {
	if !stringsHasPrefix(method, "tunnel.") {
		return params
	}
	var generic map[string]any
	if err := json.Unmarshal(params, &generic); err != nil {
		return params
	}
	redactTunnelMap(generic)
	out, err := json.Marshal(generic)
	if err != nil {
		return params
	}
	return out
}

// SanitizeTunnelRPCResult redacts sensitive tunnel.* RPC result fields before audit/logging.
func SanitizeTunnelRPCResult(method string, result json.RawMessage) json.RawMessage {
	if !stringsHasPrefix(method, "tunnel.") {
		return result
	}
	var generic map[string]any
	if err := json.Unmarshal(result, &generic); err != nil {
		return result
	}
	redactTunnelMap(generic)
	out, err := json.Marshal(generic)
	if err != nil {
		return result
	}
	return out
}

func redactTunnelMap(m map[string]any) {
	for key := range m {
		switch key {
		case "dataBase64":
			m[key] = "<redacted>"
		}
	}
}
