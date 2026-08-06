package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeTunnelRPCParamsRedactsDataBase64(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"localConnId": "lc-1",
		"dataBase64":  "c2VjcmV0",
	})
	out := SanitizeTunnelRPCParams("tunnel.localWrite", raw)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["dataBase64"] != "<redacted>" {
		t.Fatalf("dataBase64 = %v", parsed["dataBase64"])
	}
	if parsed["localConnId"] != "lc-1" {
		t.Fatalf("localConnId changed: %v", parsed["localConnId"])
	}
}

func TestSanitizeTunnelRPCParamsLeavesNonTunnelUntouched(t *testing.T) {
	raw := json.RawMessage(`{"dataBase64":"c2VjcmV0"}`)
	out := SanitizeTunnelRPCParams("vault.getSecret", raw)
	if string(out) != string(raw) {
		t.Fatalf("non-tunnel params changed: %s", out)
	}
}

func TestSanitizeTunnelRPCResultInvalidJSONPassthrough(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizeTunnelRPCResult("tunnel.localFrame", raw)
	if string(out) != string(raw) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}

func TestSanitizeTunnelRPCParamsRedactsNestedDataBase64(t *testing.T) {
	raw := json.RawMessage(`{"frames":[{"dataBase64":"c2VjcmV0"}]}`)
	out := SanitizeTunnelRPCParams("tunnel.localWrite", raw)
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("nested tunnel payload leaked: %s", out)
	}
}

// The two key sets stay separate: sharing a redactor must not merge what each
// protocol considers sensitive.
func TestSanitizeTunnelRPCParamsLeavesAuthOnlyKeysAlone(t *testing.T) {
	raw := json.RawMessage(`{"signatureBase64":"c2ln"}`)
	out := SanitizeTunnelRPCParams("tunnel.localWrite", raw)
	if !strings.Contains(string(out), "c2ln") {
		t.Fatalf("tunnel sanitizer redacted an auth-only key: %s", out)
	}
}
