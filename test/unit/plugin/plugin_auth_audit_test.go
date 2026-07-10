package plugin_test

import (
	"encoding/json"
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestSanitizeAuthRPCParamsAuditPayload(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{
		"attemptId": "a1", "dataBase64": "c2VjcmV0cGFzc3dvcmQ=",
	})
	sanitized := string(domainplugin.SanitizeAuthRPCParams("auth.sign", raw))
	if strings.Contains(sanitized, "c2VjcmV0") {
		t.Fatalf("audit payload must not contain raw base64: %s", sanitized)
	}
	if !strings.Contains(sanitized, "redacted") {
		t.Fatalf("expected redacted marker in %s", sanitized)
	}
}
