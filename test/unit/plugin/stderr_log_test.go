package plugin_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/infra/loghub"
	infraplugin "xquakshell/internal/infra/plugin"
)

func TestRedactingStderrWriterRedactsSecrets(t *testing.T) {
	// Plugin stderr is published once into the loghub (source plugin-stderr:<id>),
	// no longer mirrored through slog. Subscribe to the hub and assert redaction.
	//
	// The level is set explicitly because PublishPluginStderr gates on Info and the host default
	// is quieter than that, so at the default nothing is published and this would time out rather
	// than fail on redaction. Redaction itself happens before publication and does not depend on
	// the level; what the level decides is only whether there is an entry to inspect.
	loghub.SetLevel(slog.LevelInfo)
	t.Cleanup(func() { loghub.SetLevel(loghub.DefaultLevel) })

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
