package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// collectingAuditRepo keeps what was appended so a test can read it back.
type collectingAuditRepo struct {
	domain.AuditLogRepository
	entries []domain.AuditEntry
	err     error
}

func (r *collectingAuditRepo) Append(_ context.Context, entry domain.AuditEntry) error {
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, entry)
	return nil
}

// The audit database is readable while the vault is locked - that is the point of it. Anything
// derived from the recovery key stored there would be an offline verifier for guessing it, so the
// entry carries the event name and nothing else.
func TestRecoveryAuditRecordsNothingDerivedFromTheKey(t *testing.T) {
	repo := &collectingAuditRepo{}
	recorder := NewRecoveryAuditRecorder(repo)

	for _, event := range []string{RecoveryEventGenerated, RecoveryEventUsed, RecoveryEventRevoked} {
		recorder.RecordRecoveryEvent(context.Background(), event)
	}

	if len(repo.entries) != 3 {
		t.Fatalf("appended %d entries, want 3", len(repo.entries))
	}
	for i, entry := range repo.entries {
		if entry.Category != domain.AuditCategorySystem {
			t.Errorf("entry %d category = %q, want %q", i, entry.Category, domain.AuditCategorySystem)
		}
		if !strings.HasPrefix(entry.Input, "vault.recovery-key.") {
			t.Errorf("entry %d input = %q, want a vault.recovery-key event", i, entry.Input)
		}
		// The event name is the whole payload. Any separator would mean something was appended to
		// it, and the only things available to append are the key or something derived from it.
		if strings.ContainsAny(entry.Input, " =:") {
			t.Errorf("entry %d input = %q; it carries more than the event name", i, entry.Input)
		}
	}
}

// A failed audit write must not fail the vault operation it describes. Nobody should be locked out
// of their own data because a log is unwell.
func TestAFailedAuditWriteIsSurvivable(t *testing.T) {
	recorder := NewRecoveryAuditRecorder(&collectingAuditRepo{err: errors.New("disk is full")})
	recorder.RecordRecoveryEvent(context.Background(), RecoveryEventUsed)

	// A nil recorder and a nil repository are both reachable: a build whose audit database failed
	// to open still has to unlock its vault.
	var absent *RecoveryAuditRecorder
	absent.RecordRecoveryEvent(context.Background(), RecoveryEventUsed)
	NewRecoveryAuditRecorder(nil).RecordRecoveryEvent(context.Background(), RecoveryEventUsed)
}

// The three events have to stay distinct. Collapsing "used" into "generated" would erase the one
// entry that tells a user their vault was opened by someone who did not know the password.
func TestTheThreeRecoveryEventsAreDistinct(t *testing.T) {
	seen := map[string]struct{}{}
	for _, event := range []string{RecoveryEventGenerated, RecoveryEventUsed, RecoveryEventRevoked} {
		if _, dup := seen[event]; dup {
			t.Fatalf("%q is used for more than one event", event)
		}
		seen[event] = struct{}{}
	}
}
