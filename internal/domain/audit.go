package domain

import (
	"context"
	"time"
)

// Audit entry categories distinguish user-entered console commands from
// program-behavior (plugin/security) audit events.
const (
	// AuditCategoryCommand marks a user-submitted terminal command.
	AuditCategoryCommand = "command"
	// AuditCategorySystem marks a program-behavior audit event (e.g. plugin security events).
	AuditCategorySystem = "system"
)

// AuditEntry represents a single logged terminal input event.
type AuditEntry struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Category       string    `json:"category"`
	SessionID      string    `json:"sessionId"`
	ConnectionID   string    `json:"connectionId"`
	ConnectionName string    `json:"connectionName"`
	Host           string    `json:"host"`
	Username       string    `json:"username"`
	Input          string    `json:"input"`
	Redacted       bool      `json:"redacted"`
}

// AuditSearchFilter provides optional filters for audit log queries.
type AuditSearchFilter struct {
	Category     string     `json:"category,omitempty"`
	SessionID    string     `json:"sessionId,omitempty"`
	ConnectionID string     `json:"connectionId,omitempty"`
	From         *time.Time `json:"from,omitempty"`
	To           *time.Time `json:"to,omitempty"`
	Limit        int        `json:"limit,omitempty"`
	Offset       int        `json:"offset,omitempty"`
}

// AuditLogRepository persists and queries terminal input audit events.
type AuditLogRepository interface {
	// Append writes a new audit entry to the log.
	Append(ctx context.Context, entry AuditEntry) error
	// Search performs full-text search on audit entries with optional filters.
	Search(ctx context.Context, query string, filter AuditSearchFilter) ([]AuditEntry, error)
	// DeleteByID removes a single audit entry by ID.
	DeleteByID(ctx context.Context, id int64) error
	// ClearAll removes audit entries. An empty category clears all entries;
	// a non-empty category clears only entries of that category.
	ClearAll(ctx context.Context, category string) error
	// Count returns the total number of audit entries.
	Count(ctx context.Context) (int64, error)
	// PurgeOlderThan deletes entries with timestamp strictly before cutoff.
	PurgeOlderThan(ctx context.Context, cutoff time.Time) error
	// TrimToCount deletes oldest entries until at most max remain.
	TrimToCount(ctx context.Context, max int) error
	// Close releases underlying storage resources.
	Close() error
}
