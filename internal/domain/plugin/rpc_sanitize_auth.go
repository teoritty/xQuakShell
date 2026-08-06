package plugin

import (
	"encoding/json"
)

// sensitiveAuthKeys are the auth.* payload fields that must never reach the
// audit log. "answers" and "dataBase64" carry what the user typed at a
// keyboard-interactive prompt; the two Base64 blobs carry signing material.
// A question's prompt text is redacted too, but only inside questions[] —
// see isSensitiveKey.
var sensitiveAuthKeys = map[string]struct{}{
	"answers":             {},
	"dataBase64":          {},
	"signatureBase64":     {},
	"publicKeyBlobBase64": {},
}

// SanitizeAuthRPCParams redacts sensitive auth.* RPC payload fields before audit/logging.
func SanitizeAuthRPCParams(method string, params json.RawMessage) json.RawMessage {
	return sanitizeRPC("auth.", method, params, sensitiveAuthKeys)
}

// SanitizeAuthRPCResult redacts sensitive auth.* RPC result fields before audit/logging.
func SanitizeAuthRPCResult(method string, result json.RawMessage) json.RawMessage {
	return sanitizeRPC("auth.", method, result, sensitiveAuthKeys)
}
