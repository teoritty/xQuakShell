package auditlog

import domainplugin "ssh-client/internal/domain/plugin"

// RedactPluginLogMessage removes likely secrets from plugin log lines.
func RedactPluginLogMessage(message string) (redacted string, changed bool) {
	return domainplugin.RedactLogMessage(message)
}

// RedactPluginAuditDetail sanitizes audit detail strings from plugin RPC denials.
func RedactPluginAuditDetail(detail string) string {
	return domainplugin.RedactAuditDetail(detail)
}
