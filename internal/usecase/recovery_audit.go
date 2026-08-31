package usecase

import (
	"context"
	"log/slog"

	"xquakshell/internal/domain"
)

const (
	// RecoveryEventGenerated is a key being minted, whether at vault creation, after a reset, or
	// because the user asked for a new one.
	RecoveryEventGenerated = "vault.recovery-key.generated"

	// RecoveryEventUsed is the one that matters. A vault opened with the recovery key was opened by
	// someone who did not know the master password - which is the owner on a bad day, or is not the
	// owner at all. It is the only way a user ever finds out that the paper in the drawer was read.
	RecoveryEventUsed = "vault.recovery-key.used"

	// RecoveryEventRevoked is an old key being retired, by rotation or by a password change.
	RecoveryEventRevoked = "vault.recovery-key.revoked"
)

// RecoveryAuditRecorder writes recovery key events to the audit log.
//
// It records the event name and nothing else: no key, no hash of one, no salt. A hash would be a
// verifier for an offline guess, and the audit database is not held to the vault's standard - it is
// readable while the vault is locked, which is the whole point of it.
type RecoveryAuditRecorder struct {
	repo domain.AuditLogRepository
}

// NewRecoveryAuditRecorder returns a recorder writing to repo. A nil repo drops events, because a
// build whose audit database failed to open must still be able to unlock its vault.
func NewRecoveryAuditRecorder(repo domain.AuditLogRepository) *RecoveryAuditRecorder {
	return &RecoveryAuditRecorder{repo: repo}
}

// RecordRecoveryEvent appends one recovery key event.
func (r *RecoveryAuditRecorder) RecordRecoveryEvent(ctx context.Context, event string) {
	if r == nil || r.repo == nil {
		return
	}
	if err := r.repo.Append(ctx, domain.AuditEntry{
		Category: domain.AuditCategorySystem,
		Input:    event,
	}); err != nil {
		// A failed audit write must not fail the vault operation it describes: nobody should be
		// locked out of their own data because a log is unwell. Warn so the gap is visible.
		slog.Warn("recovery audit write failed", "event", event, "err", err)
	}
}
