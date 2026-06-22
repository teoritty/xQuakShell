package domain

// AuditRetentionMode controls how audit log entries are pruned.
type AuditRetentionMode string

const (
	AuditRetentionByDays  AuditRetentionMode = "days"
	AuditRetentionByCount AuditRetentionMode = "count"
)

// AuditLogSettings configures command audit logging behaviour.
type AuditLogSettings struct {
	Enabled        bool               `json:"enabled"`
	RetentionMode  AuditRetentionMode `json:"retentionMode"`
	RetentionDays  int                `json:"retentionDays"`
	RetentionCount int                `json:"retentionCount"`
	ShowUsername   bool               `json:"showUsername"`
	ShowConnection bool               `json:"showConnection"`
}

// DefaultAuditLogSettings returns opt-in defaults: disabled, 30-day retention.
func DefaultAuditLogSettings() AuditLogSettings {
	return AuditLogSettings{
		Enabled:        false,
		RetentionMode:  AuditRetentionByDays,
		RetentionDays:  30,
		RetentionCount: 100,
		ShowUsername:   false,
		ShowConnection: false,
	}
}
