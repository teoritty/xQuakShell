package plugin

import (
	"encoding/json"
	"strings"
)

// The redaction primitive shared by every RPC sanitizer.
//
// Each protocol used to carry its own copy of "unmarshal, walk, marshal" plus a
// hand-written walker that enumerated the shapes it expected. Enumeration is the
// wrong shape for this job twice over: the walkers only ever reached the depth
// someone had thought of, so a sensitive key nested one level lower than the
// examples reached the audit log in clear text; and two copies of an
// authorization-adjacent rule drift apart the moment one of them is fixed.
//
// What varies between protocols is a method prefix and a set of key names.
// Everything else lives here once.

const redactedMarker = "<redacted>"

// maxRedactDepth bounds the walk. No real RPC payload approaches it; a hostile
// one is not allowed to turn recursion into a stack overflow.
const maxRedactDepth = 64

// sanitizeRPC redacts sensitive keys in an RPC payload before it is audited or
// logged. Payloads for other methods, payloads that are not JSON, and payloads
// that are not a JSON object are returned byte for byte unchanged: this
// function makes a payload safer to record, never harder to read back.
func sanitizeRPC(prefix, method string, payload json.RawMessage, sensitive map[string]struct{}) json.RawMessage {
	if !strings.HasPrefix(method, prefix) {
		return payload
	}
	var generic map[string]any
	if err := json.Unmarshal(payload, &generic); err != nil {
		return payload
	}
	out, err := json.Marshal(redactValue(generic, sensitive, "", 0))
	if err != nil {
		return payload
	}
	return out
}

// redactValue returns v with every sensitive key replaced, at any depth.
//
// parentKey carries the key that led to v so a rule can be scoped to one branch
// of the payload; it is threaded through slices unchanged so the elements of
// questions[] still know they are questions. Redacting a key as generic as
// "text" everywhere would empty the audit log of the context that makes it
// worth keeping.
func redactValue(v any, sensitive map[string]struct{}, parentKey string, depth int) any {
	if depth > maxRedactDepth {
		// Fail closed: past the bound we can no longer account for what is
		// below, and an unreadable audit entry beats a leaked one.
		return redactedMarker
	}
	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			if isSensitiveKey(key, parentKey, sensitive) {
				typed[key] = redactedMarker
				continue
			}
			typed[key] = redactValue(value, sensitive, key, depth+1)
		}
		return typed
	case []any:
		for i, item := range typed {
			typed[i] = redactValue(item, sensitive, parentKey, depth+1)
		}
		return typed
	default:
		return v
	}
}

// isSensitiveKey answers whether a single field must be redacted. The scoped
// rule is deliberately narrow: a question's prompt text is sensitive because a
// keyboard-interactive prompt can name the secret it is asking for, but "text"
// anywhere else in a payload is not.
func isSensitiveKey(key, parentKey string, sensitive map[string]struct{}) bool {
	if _, ok := sensitive[key]; ok {
		return true
	}
	return parentKey == "questions" && key == "text"
}
