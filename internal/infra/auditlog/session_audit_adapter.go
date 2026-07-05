package auditlog

import domainplugin "ssh-client/internal/domain/plugin"

// RecordSessionAudit implements domainplugin.SessionAuditor.
func (l *PluginSessionAuditLog) RecordSessionAudit(entry domainplugin.SessionAuditEntry) {
	if l == nil {
		return
	}
	l.Record(PluginSessionAuditEntry{
		Timestamp:    entry.Timestamp,
		PluginID:     entry.PluginID,
		Action:       entry.Action,
		ConnectionID: entry.ConnectionID,
		Protocol:     entry.Protocol,
		FieldCount:   entry.FieldCount,
		Success:      entry.Success,
		Error:        entry.Error,
	})
}
