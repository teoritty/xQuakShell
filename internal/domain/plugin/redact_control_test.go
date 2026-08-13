package plugin

import (
	"strings"
	"testing"
)

// A plugin's stderr reaches three places: the log window, slog, and the process stderr a developer
// or a CI job reads. The window renders it as Svelte text so markup is inert there - but a terminal
// honours what it is sent. A carriage return rewrites the line already printed and an ANSI escape
// clears the screen, so a plugin could compose its own log line and erase the one above it.
func TestRedactLogMessageStripsTerminalControlSequences(t *testing.T) {
	tests := []struct {
		name    string
		message string
		absent  string
	}{
		{"carriage return rewrites the printed line", "benign\r[INFO] vault unlocked", "\r"},
		{"ANSI clear screen", "quiet\x1b[2Jnothing to see", "\x1b"},
		{"ANSI cursor movement", "line\x1b[1Aoverwritten", "\x1b"},
		{"backspace erases characters", "denied\x08\x08\x08\x08\x08\x08allowed", "\x08"},
		{"NUL truncates for a C reader", "before\x00after", "\x00"},
		{"DEL", "a\x7fb", "\x7f"},
		{"C1 escape as a single byte", "amb", ""},
		{"vertical tab and form feed", "a\x0bb\x0cc", "\x0b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := RedactLogMessage(tt.message)
			if strings.Contains(got, tt.absent) {
				t.Errorf("RedactLogMessage(%q) = %q; it still carries %q, which a terminal obeys", tt.message, got, tt.absent)
			}
			if !changed {
				t.Errorf("RedactLogMessage(%q) reported no change while stripping control characters", tt.message)
			}
		})
	}
}

// Stripping must not eat the message. A log line with its control characters removed is still the
// line the plugin wrote, and a reader has to be able to see what was said.
func TestRedactLogMessageKeepsTheTextAroundTheControls(t *testing.T) {
	got, _ := RedactLogMessage("connect failed\x1b[31m: timeout")

	for _, want := range []string{"connect failed", "timeout"} {
		if !strings.Contains(got, want) {
			t.Errorf("RedactLogMessage dropped %q from the line: got %q", want, got)
		}
	}
}

// Tab is layout a plugin legitimately uses to align its own output, and no renderer treats it as a
// command. Stripping it would mangle ordinary logs for nothing.
func TestRedactLogMessageKeepsTabs(t *testing.T) {
	got, changed := RedactLogMessage("field\tvalue")

	if got != "field\tvalue" {
		t.Errorf("RedactLogMessage = %q, want the tab preserved", got)
	}
	if changed {
		t.Error("a line containing only a tab was reported as modified")
	}
}

// Non-ASCII text is not a control character. A plugin logging in Cyrillic, CJK or with emoji must
// come through intact, or the strip becomes a second bug reported as garbled output.
func TestRedactLogMessageKeepsNonASCIIText(t *testing.T) {
	const message = "плагин запущен · 插件已启动 · ✅"

	got, changed := RedactLogMessage(message)

	if got != message {
		t.Errorf("RedactLogMessage = %q, want %q unchanged", got, message)
	}
	if changed {
		t.Error("ordinary non-ASCII text was reported as modified")
	}
}

// The control strip runs alongside the secret patterns and must not disturb them: a secret split by
// an escape sequence still has to be caught once the escape is gone.
func TestRedactLogMessageStillRedactsSecretsAfterStripping(t *testing.T) {
	got, changed := RedactLogMessage("password: \x1b[0mhunter2")

	if strings.Contains(got, "hunter2") {
		t.Errorf("RedactLogMessage = %q; the secret survived", got)
	}
	if !changed {
		t.Error("a redacted secret was reported as no change")
	}
}
