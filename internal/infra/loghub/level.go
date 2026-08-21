package loghub

import (
	"log/slog"
	"strings"
	"sync/atomic"
)

// DefaultLevel is the level the process runs at until settings override it, and the level an
// unset or unrecognised setting resolves to.
//
// WHY NOT DEBUG. This constant is not a verbosity preference, it is the price of every log
// statement in the process, including the ones on data paths that run per frame. The embed tunnel
// emits five records per video frame across the broker, the tunnel service and the channel
// backend; at Debug each costs ~2 us and ~750 B and takes Hub.mu, which is one global mutex for
// the whole process. Gating the same statements out costs ~34 ns and one allocation — measured,
// on the real handler. Nobody had chosen Debug either: DebugSettings.LogLevel is `omitempty`, so
// every install that never opened the Developer settings resolved an empty string through
// ParseLevel's fallback and got the most expensive level by accident.
//
// WHY WARN AND NOT INFO. Info still publishes plugin stderr (PublishPluginStderr gates on it) and
// every business event, which on a busy session is the same failure mode one order of magnitude
// down. Warn keeps what a user can act on — recoverable failures — and nothing that scales with
// traffic.
//
// The verbose levels are one click away in Developer settings and are what a bug report asks for.
// This decides only what an install costs when nobody has asked for anything.
const DefaultLevel = slog.LevelWarn

// currentLevel holds the process-wide minimum log level as an atomic int32.
// Records below this level are dropped before formatting. Reads are lock-free
// so the hot path stays contention-free.
var currentLevel atomic.Int32

func init() {
	currentLevel.Store(int32(DefaultLevel))
}

func SetLevel(level slog.Level) {
	// #nosec G115 -- slog.Level is a small int constant set (Debug=-4 … Error=8).
	currentLevel.Store(int32(level))
}

func Level() slog.Level {
	return slog.Level(currentLevel.Load())
}

func Enabled(level slog.Level) bool {
	return level >= Level()
}

// ParseLevel resolves a settings level name. "debug" needs a case of its own rather than riding
// the fallback: the fallback answers for an empty or unrecognised name, and those two must never
// resolve to a more expensive level than the one an install was going to get anyway.
func ParseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return DefaultLevel
	}
}
