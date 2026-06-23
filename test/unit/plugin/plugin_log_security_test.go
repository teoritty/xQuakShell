package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func TestSanitizeLogWriteParamsRedactsSensitiveFields(t *testing.T) {
	raw, _ := json.Marshal(domainplugin.LogWriteParams{
		Level:   "info",
		Message: "connected",
		Fields: map[string]string{
			"password": "hunter2",
			"host":     "example.com",
		},
	})
	payload, changed := domainplugin.SanitizeLogWriteParams(raw)
	if !changed {
		t.Fatal("expected redaction")
	}
	if payload.Fields["password"] != "[REDACTED]" {
		t.Fatalf("expected password redacted, got %q", payload.Fields["password"])
	}
	if payload.Fields["host"] != "example.com" {
		t.Fatalf("expected host preserved, got %q", payload.Fields["host"])
	}
}

func TestSanitizeLogWriteParamsRedactsFreeTextSecrets(t *testing.T) {
	raw, _ := json.Marshal(domainplugin.LogWriteParams{
		Level:   "error",
		Message: "login failed password=sekret",
	})
	_, changed := domainplugin.SanitizeLogWriteParams(raw)
	if !changed {
		t.Fatal("expected free-text secret redaction")
	}
}

func TestHostServerStructuredLogWrite(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate:     capability.NewGate(domainplugin.Manifest{}),
	})
	params, _ := json.Marshal(domainplugin.LogWriteParams{
		Level:   "info",
		Message: "event",
		Fields:  map[string]string{"token": "abc123"},
	})
	if _, rpcErr := server.HandleRequest(context.Background(), "log.write", params); rpcErr != nil {
		t.Fatalf("log.write failed: %#v", rpcErr)
	}
}
