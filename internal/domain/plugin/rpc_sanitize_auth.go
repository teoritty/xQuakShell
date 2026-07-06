package plugin

import (
	"encoding/json"
	"strings"
)

// SanitizeAuthRPCParams redacts sensitive auth.* RPC payload fields before audit/logging.
func SanitizeAuthRPCParams(method string, params json.RawMessage) json.RawMessage {
	if !stringsHasPrefix(method, "auth.") {
		return params
	}
	var generic map[string]any
	if err := json.Unmarshal(params, &generic); err != nil {
		return params
	}
	redactAuthMap(generic)
	out, err := json.Marshal(generic)
	if err != nil {
		return params
	}
	return out
}

// SanitizeAuthRPCResult redacts sensitive auth.* RPC result fields before audit/logging.
func SanitizeAuthRPCResult(method string, result json.RawMessage) json.RawMessage {
	if !stringsHasPrefix(method, "auth.") {
		return result
	}
	var generic map[string]any
	if err := json.Unmarshal(result, &generic); err != nil {
		return result
	}
	redactAuthMap(generic)
	out, err := json.Marshal(generic)
	if err != nil {
		return result
	}
	return out
}

func redactAuthMap(m map[string]any) {
	for key, value := range m {
		switch key {
		case "answers", "dataBase64", "signatureBase64", "publicKeyBlobBase64":
			m[key] = "<redacted>"
		case "questions":
			if qs, ok := value.([]any); ok {
				for _, item := range qs {
					if qm, ok := item.(map[string]any); ok {
						if _, ok := qm["text"]; ok {
							qm["text"] = "<redacted>"
						}
					}
				}
			}
		}
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}
