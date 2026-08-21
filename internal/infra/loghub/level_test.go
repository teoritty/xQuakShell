package loghub

import (
	"log/slog"
	"testing"
)

// The level the process runs at before any settings are loaded decides the cost of every log
// statement on a data path. Debug there made the embed tunnel pay for five records per video
// frame — measured at ~2 us and ~750 B each, all serialised on Hub.mu — because DebugSettings
// .LogLevel is `omitempty` and an empty value resolves through ParseLevel's fallback, so no user
// ever chose it. Gating those same statements out costs ~34 ns and one allocation.
//
// The assertion is deliberately "quieter than Info" rather than "equals Warn": what must never
// come back is a default that publishes per-frame records, and pinning the exact constant would
// make a later move to Error look like a regression.
func TestDefaultLevelIsQuieterThanInfo(t *testing.T) {
	if DefaultLevel <= slog.LevelInfo {
		t.Fatalf("DefaultLevel = %v, want a level above Info; a default that publishes Debug or "+
			"Info records makes every per-frame log statement cost the hot path", DefaultLevel)
	}
}

// init() and ParseLevel are two independent doors to the same decision — the process starts at
// one, and an unset or unrecognised setting resolves through the other. They disagreed once
// already, which is how a UI showing "debug" ran against a process that was not at debug.
func TestUnsetLevelResolvesToTheSameDefaultTheProcessStartsAt(t *testing.T) {
	defer SetLevel(DefaultLevel)

	if got := Level(); got != DefaultLevel {
		t.Fatalf("level at process start = %v, want %v", got, DefaultLevel)
	}
	for _, name := range []string{"", "   ", "not-a-real-level"} {
		if got := ParseLevel(name); got != DefaultLevel {
			t.Errorf("ParseLevel(%q) = %v, want the host default %v", name, got, DefaultLevel)
		}
	}
}

func TestParseLevelAcceptsEveryNameTheSettingsDialogOffers(t *testing.T) {
	for name, want := range map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"  WARN ": slog.LevelWarn,
	} {
		if got := ParseLevel(name); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", name, got, want)
		}
	}
}
