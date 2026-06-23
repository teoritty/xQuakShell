package auditlog_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/auditlog"
)

func TestNDJSONVaultAuditLoggerRecordsAccessWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	logger, err := auditlog.NewNDJSONVaultAuditLogger(dir)
	if err != nil {
		t.Fatal(err)
	}

	secret := "super-secret-password"
	err = logger.RecordVaultAccess(context.Background(), domainplugin.VaultAccessEvent{
		Timestamp:    time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
		PluginID:     "com.test.plugin",
		ConnectionID: "conn-1",
		Method:       domainplugin.VaultAccessGetSecret,
		Field:        "password",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "audit", "plugin-vault.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "vault.getSecret") {
		t.Fatalf("expected method in audit log, got %q", content)
	}
	if strings.Contains(content, secret) {
		t.Fatalf("audit log leaked secret value: %q", content)
	}

	var record struct {
		PluginID     string `json:"pluginId"`
		ConnectionID string `json:"connectionId"`
		Field        string `json:"field"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &record); err != nil {
		t.Fatal(err)
	}
	if record.PluginID != "com.test.plugin" || record.ConnectionID != "conn-1" || record.Field != "password" {
		t.Fatalf("unexpected record %+v", record)
	}
}
