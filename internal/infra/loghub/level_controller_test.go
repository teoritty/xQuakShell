package loghub

import (
	"log/slog"
	"testing"
)

func TestLevelControllerSetLevel(t *testing.T) {
	defer SetLevel(slog.LevelDebug)

	var c LevelController
	c.SetLevel("warn")
	if Level() != slog.LevelWarn {
		t.Fatalf("expected warn level, got %v", Level())
	}

	c.SetLevel("error")
	if Level() != slog.LevelError {
		t.Fatalf("expected error level, got %v", Level())
	}
}

func TestLevelControllerSetLevelUnknownDefaultsToDebug(t *testing.T) {
	defer SetLevel(slog.LevelDebug)

	var c LevelController
	c.SetLevel("warn")
	c.SetLevel("not-a-real-level")
	if Level() != slog.LevelDebug {
		t.Fatalf("expected unknown input to default to debug, got %v", Level())
	}
}
