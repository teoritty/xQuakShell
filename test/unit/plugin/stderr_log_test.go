package plugin_test

import (
	"strings"
	"testing"
	"time"

	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/loghub"
)

func TestRedactingStderrWriterRedactsSecrets(t *testing.T) {
	// Plugin stderr is published once into the loghub (source plugin-stderr:<id>),
	// no longer mirrored through slog. Subscribe to the hub and assert redaction.
	id, _, ch := loghub.Default().Subscribe(16)
	t.Cleanup(func() { loghub.Default().Unsubscribe(id) })

	writer := infraplugin.NewRedactingStderrWriter("com.test.plugin")
	if _, err := writer.Write([]byte("password=hunter2\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			if !strings.HasPrefix(e.Source, "plugin-stderr:") {
				continue
			}
			if strings.Contains(e.Message, "hunter2") {
				t.Fatalf("expected secret redacted from stderr log, got %q", e.Message)
			}
			if !strings.Contains(e.Message, "REDACTED") && e.Fields["redacted"] != "true" {
				t.Fatalf("expected redaction marker in stderr log, got message=%q fields=%v", e.Message, e.Fields)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for stderr log entry")
		}
	}
}
