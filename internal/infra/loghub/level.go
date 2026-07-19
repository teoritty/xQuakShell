package loghub

import (
	"log/slog"
	"strings"
	"sync/atomic"
)

// currentLevel holds the process-wide minimum log level as an atomic int32.
// Records below this level are dropped before formatting. Reads are lock-free
// so the hot path stays contention-free.
var currentLevel atomic.Int32

func init() {
	// Default to Debug so the log is lively out of the box; the UI level filter
	// and the Debug.LogLevel setting let the user raise the floor.
	currentLevel.Store(int32(slog.LevelDebug))
}

// SetLevel sets the process-wide minimum log level.
func SetLevel(level slog.Level) {
	currentLevel.Store(int32(level))
}

// Level returns the current process-wide minimum log level.
func Level() slog.Level {
	return slog.Level(currentLevel.Load())
}

// Enabled reports whether the given level passes the current floor.
func Enabled(level slog.Level) bool {
	return level >= Level()
}

// ParseLevel maps a case-insensitive level name to a slog.Level, defaulting to
// Debug for empty or unrecognized input.
func ParseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
