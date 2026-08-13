package plugin

import (
	"regexp"
	"strings"
)

var (
	pluginPasswordPattern   = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key|private[_-]?key)\s*[:=]\s*\S+`)
	pluginBearerPattern     = regexp.MustCompile(`(?i)bearer\s+\S+`)
	pluginPEMPattern        = regexp.MustCompile(`-----BEGIN [A-Z ]+-----[\s\S]*?-----END [A-Z ]+-----`)
	pluginBase64Pattern     = regexp.MustCompile(`(?i)(contentBase64|dataBase64|outputBase64|secretBase64|payloadBase64|valueBase64)\s*[:=]\s*["']?[A-Za-z0-9+/=]{32,}`)
	pluginConnSecretPattern = regexp.MustCompile(`(?i)(connectionId|sessionId)\s*[:=]\s*["']?[A-Za-z0-9._-]{8,}`)
)

// RedactLogMessage sanitises a plugin log line: likely secrets are removed, control characters are
// stripped, and the result is bounded in length.
//
// The control characters matter for a reason the secret patterns do not cover. This text is plugin
// output and it reaches three places: the log window, slog, and the process stderr a developer or a
// CI job reads. The window renders it as Svelte text, so markup is inert there - but a terminal
// honours what it is sent. A carriage return rewrites the line already printed, and an ANSI escape
// moves the cursor or clears the screen, so a plugin could compose its own log line saying whatever
// it liked and erase the one above it. A log a plugin can edit is not a log.
func RedactLogMessage(message string) (redacted string, changed bool) {
	redacted = message
	if stripped := stripControlChars(redacted); stripped != redacted {
		redacted = stripped
		changed = true
	}
	for _, re := range []*regexp.Regexp{
		pluginPasswordPattern,
		pluginBearerPattern,
		pluginPEMPattern,
		pluginBase64Pattern,
		pluginConnSecretPattern,
	} {
		if re.MatchString(redacted) {
			redacted = re.ReplaceAllString(redacted, "[REDACTED]")
			changed = true
		}
	}
	if len(redacted) > 512 {
		redacted = redacted[:512] + "…"
		changed = true
	}
	redacted = strings.TrimSpace(redacted)
	return redacted, changed
}

// RedactAuditDetail sanitizes audit detail strings from plugin RPC events.
func RedactAuditDetail(detail string) string {
	redacted, _ := RedactLogMessage(detail)
	return redacted
}

// stripControlChars removes the C0 and C1 control characters from a log line, leaving tab.
//
// Tab survives because it is layout a plugin legitimately uses and no renderer treats it as a
// command. Everything else in those ranges is an instruction to a terminal rather than text -
// including DEL and the C1 block, which some terminals accept as an alternative escape encoding, so
// dropping ESC alone would leave the same capability reachable by another spelling.
//
// Newlines are not special-cased: a line arriving here has already been split on one, and a
// plugin that embeds a bare CR or LF is asking to look like two log entries.
func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return r
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		default:
			return r
		}
	}, s)
}
