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

// RedactLogMessage removes likely secrets from plugin log lines.
func RedactLogMessage(message string) (redacted string, changed bool) {
	redacted = message
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
