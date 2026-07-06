package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeAuthRPCParamsRedactsSensitiveFields(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"attemptId":    "a1",
		"authMethodId": "otp",
		"dataBase64":   "c2VjcmV0",
		"answers":      []string{"123456"},
		"questions": []map[string]any{
			{"text": "Password:", "echoOn": false},
		},
	})
	out := SanitizeAuthRPCParams("auth.answerChallenge", raw)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["dataBase64"] != "<redacted>" {
		t.Fatalf("dataBase64 = %v", parsed["dataBase64"])
	}
	if parsed["answers"] != "<redacted>" {
		t.Fatalf("answers = %v", parsed["answers"])
	}
	qs := parsed["questions"].([]any)
	q0 := qs[0].(map[string]any)
	if q0["text"] != "<redacted>" {
		t.Fatalf("question text = %v", q0["text"])
	}
}

func TestSanitizeAuthRPCParamsLeavesNonAuthUntouched(t *testing.T) {
	raw := json.RawMessage(`{"dataBase64":"c2VjcmV0"}`)
	out := SanitizeAuthRPCParams("vault.getSecret", raw)
	if string(out) != string(raw) {
		t.Fatalf("non-auth params changed: %s", out)
	}
}

func TestSanitizeAuthRPCResultRedactsSignature(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{
		"signatureBase64":   "c2VjcmV0",
		"signatureFormat":   "ssh-ed25519",
		"publicKeyBlobBase64": "AQID",
	})
	out := SanitizeAuthRPCResult("auth.sign", raw)
	if strings.Contains(string(out), "c2VjcmV0") {
		t.Fatalf("signature not redacted: %s", out)
	}
	if !strings.Contains(string(out), "redacted") {
		t.Fatalf("expected redacted marker in %s", out)
	}
}

func TestSanitizeAuthRPCResultInvalidJSONPassthrough(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizeAuthRPCResult("auth.sign", raw)
	if string(out) != string(raw) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}

func TestSanitizeAuthRPCResultLeavesNonAuthUntouched(t *testing.T) {
	raw := json.RawMessage(`{"signatureBase64":"c2VjcmV0"}`)
	out := SanitizeAuthRPCResult("vault.getSecret", raw)
	if string(out) != string(raw) {
		t.Fatalf("non-auth result changed: %s", out)
	}
}

func TestSanitizeAuthRPCParamsInvalidJSONPassthrough(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizeAuthRPCParams("auth.sign", raw)
	if string(out) != string(raw) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}
