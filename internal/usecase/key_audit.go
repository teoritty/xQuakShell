package usecase

import (
	"context"
	"log/slog"

	"xquakshell/internal/domain"
)

// KeyAuditRecorder writes key-management events to the audit log.
//
// It records the key by id and fingerprint and nothing else. A fingerprint is public — it is what
// the user compares against the server — so the log stays readable without ever holding key
// material, a passphrase, or anything derived from either.
type KeyAuditRecorder struct {
	repo domain.AuditLogRepository
}

// NewKeyAuditRecorder returns a recorder writing to repo. A nil repo yields a recorder that drops
// events, because a build whose audit database failed to open must still be able to manage keys —
// refusing every key operation would turn a degraded log into a broken application.
func NewKeyAuditRecorder(repo domain.AuditLogRepository) *KeyAuditRecorder {
	return &KeyAuditRecorder{repo: repo}
}

var _ KeyManagerAudit = (*KeyAuditRecorder)(nil)

// RecordKeyEvent appends one key-management event.
func (r *KeyAuditRecorder) RecordKeyEvent(ctx context.Context, event, identityID, fingerprint string) {
	if r == nil || r.repo == nil {
		return
	}
	entry := domain.AuditEntry{
		Category: domain.AuditCategorySystem,
		Input:    event + " id=" + identityID + " " + fingerprint,
	}
	if err := r.repo.Append(ctx, entry); err != nil {
		// A failed audit write must not fail the key operation it describes: the user would be
		// left unable to manage keys because a log is unwell. It is logged at warn so the gap is
		// visible rather than silent.
		slog.Warn("key audit write failed", "event", event, "id", identityID, "err", err)
	}
}
