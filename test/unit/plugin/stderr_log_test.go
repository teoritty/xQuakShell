package plugin_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	infraplugin "ssh-client/internal/infra/plugin"
)

func TestRedactingStderrWriterRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	writer := infraplugin.NewRedactingStderrWriter("com.test.plugin")
	if _, err := writer.Write([]byte("password=hunter2\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "hunter2") {
		t.Fatalf("expected secret redacted from stderr log, got %q", out)
	}
	if !strings.Contains(out, "REDACTED") && !strings.Contains(out, "redacted") {
		t.Fatalf("expected redaction marker in stderr log, got %q", out)
	}
}
