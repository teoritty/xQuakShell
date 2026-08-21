package loghub

import (
	"log/slog"
	"testing"
)

func TestLevelControllerSetLevel(t *testing.T) {
	defer SetLevel(DefaultLevel)

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

func TestLevelControllerSetLevelUnknownFallsBackToTheHostDefault(t *testing.T) {
	defer SetLevel(DefaultLevel)

	var c LevelController
	c.SetLevel("error")
	c.SetLevel("not-a-real-level")
	if Level() != DefaultLevel {
		t.Fatalf("unknown level name = %v, want the host default %v; an unrecognised "+
			"name must not switch the process to a more expensive level than the default",
			Level(), DefaultLevel)
	}
}

// The settings dialog offers "debug" and it has to keep working. It used to ride ParseLevel's
// fallback arm rather than a case of its own, so moving that fallback off Debug would have
// silently disabled the most verbose level while still showing it in the UI.
func TestLevelControllerSetLevelDebugIsStillReachable(t *testing.T) {
	defer SetLevel(DefaultLevel)

	var c LevelController
	c.SetLevel("error")
	c.SetLevel("debug")
	if Level() != slog.LevelDebug {
		t.Fatalf("explicit \"debug\" = %v, want %v", Level(), slog.LevelDebug)
	}
}
