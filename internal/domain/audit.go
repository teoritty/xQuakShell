package domain

import (
	"context"
	"time"
)

// AuditEntry represents a single logged terminal input event.
type AuditEntry struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	SessionID    string    `json:"sessionId"`
	ConnectionID string    `json:"connectionId"`
	Username     string    `json:"username"`
	Input        string    `json:"input"`
	Redacted     bool      `json:"redacted"`
}

// AuditSearchFilter provides optional filters for audit log queries.
type AuditSearchFilter struct {
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
	// ClearAll removes all audit entries.
	ClearAll(ctx context.Context) error
	// Close releases underlying storage resources.
	Close() error
}
