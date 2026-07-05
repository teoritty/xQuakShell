package plugin

import "time"

// SessionAuditEntry records a plugin session lifecycle event without secret values.
type SessionAuditEntry struct {
	Timestamp    time.Time
	PluginID     string
	Action       string
	ConnectionID string
	Protocol     string
	FieldCount   int
	Success      bool
	Error        string
}

// SessionAuditor records plugin session audit events.
type SessionAuditor interface {
	RecordSessionAudit(entry SessionAuditEntry)
}
