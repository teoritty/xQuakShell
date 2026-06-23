package plugin_test

import (
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestRedactLogMessageSecrets(t *testing.T) {
	msg := "connect password=hunter2 host=example.com"
	redacted, changed := domainplugin.RedactLogMessage(msg)
	if !changed {
		t.Fatal("expected redaction")
	}
	if strings.Contains(redacted, "hunter2") {
		t.Fatalf("secret leaked: %q", redacted)
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("expected marker in %q", redacted)
	}
}

func TestRedactLogMessageTruncatesLongLines(t *testing.T) {
	long := strings.Repeat("a", 600)
	redacted, changed := domainplugin.RedactLogMessage(long)
	if !changed {
		t.Fatal("expected truncation")
	}
	if len(redacted) > 520 {
		t.Fatalf("expected truncated output, got len %d", len(redacted))
	}
}

func TestRedactAuditDetail(t *testing.T) {
	detail := domainplugin.RedactAuditDetail("token=abc123secretvalue")
	if strings.Contains(detail, "abc123secretvalue") {
		t.Fatalf("audit detail not redacted: %q", detail)
	}
}
