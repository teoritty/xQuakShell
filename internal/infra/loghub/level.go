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
	currentLevel.Store(int32(slog.LevelDebug))
}

func SetLevel(level slog.Level) {
	currentLevel.Store(int32(level))
}

func Level() slog.Level {
	return slog.Level(currentLevel.Load())
}

func Enabled(level slog.Level) bool {
	return level >= Level()
}

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
