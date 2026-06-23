package plugin

import (
	"encoding/json"
	"strings"
)

// Sensitive log field keys that must never be forwarded to host logs or audit.
var sensitiveLogFieldKeys = map[string]struct{}{
	"password":    {},
	"passwd":      {},
	"secret":      {},
	"token":       {},
	"apikey":      {},
	"api_key":     {},
	"privatekey":  {},
	"private_key": {},
	"key":         {},
	"passphrase":  {},
	"credential":  {},
	"auth":        {},
}

// LogWriteParams is the structured plugin log.write RPC payload.
type LogWriteParams struct {
	Level   string            `json:"level"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// SanitizeLogWriteParams redacts sensitive structured fields and free-text message patterns.
func SanitizeLogWriteParams(params json.RawMessage) (LogWriteParams, bool) {
	var req LogWriteParams
	if err := json.Unmarshal(params, &req); err != nil {
		return LogWriteParams{}, false
	}
	changed := false
	if len(req.Fields) > 0 {
		safe := make(map[string]string, len(req.Fields))
		for key, value := range req.Fields {
			if isSensitiveLogFieldKey(key) {
				safe[key] = "[REDACTED]"
				changed = true
				continue
			}
			redacted, fieldChanged := RedactLogMessage(value)
			if fieldChanged {
				changed = true
			}
			safe[key] = redacted
		}
		req.Fields = safe
	}
	if req.Message != "" {
		redacted, msgChanged := RedactLogMessage(req.Message)
		if msgChanged {
			changed = true
		}
		req.Message = redacted
	}
	return req, changed
}

func isSensitiveLogFieldKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if _, ok := sensitiveLogFieldKeys[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "private_key") ||
		strings.Contains(normalized, "privatekey")
}
