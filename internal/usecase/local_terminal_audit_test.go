package usecase

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

type recordingAuditRepo struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
	err     error
}

func (r *recordingAuditRepo) Append(_ context.Context, entry domain.AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return r.err
}

func (r *recordingAuditRepo) recorded() []domain.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.AuditEntry(nil), r.entries...)
}

// The rest of the interface is unused here. It is spelled out rather than embedded so that a
// change to the audit port shows up as a compile error in this file, where a maintainer can see
// which fake needs updating.
func (r *recordingAuditRepo) Search(context.Context, string, domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	return nil, nil
}
func (r *recordingAuditRepo) DeleteByID(context.Context, int64) error         { return nil }
func (r *recordingAuditRepo) ClearAll(context.Context, string) error          { return nil }
func (r *recordingAuditRepo) Count(context.Context) (int64, error)            { return 0, nil }
func (r *recordingAuditRepo) PurgeOlderThan(context.Context, time.Time) error { return nil }
func (r *recordingAuditRepo) TrimToCount(context.Context, int) error          { return nil }
func (r *recordingAuditRepo) Close() error                                    { return nil }

var _ domain.AuditLogRepository = (*recordingAuditRepo)(nil)

func TestAuditRecordsOpenAndCloseWithTheShellName(t *testing.T) {
	repo := &recordingAuditRepo{}
	rec := NewLocalTerminalAuditRecorder(repo)

	rec.Opened("lt-1", "pwsh")
	rec.Closed("lt-1", "pwsh")

	got := repo.recorded()
	if len(got) != 2 {
		t.Fatalf("recorded %d entries, want 2", len(got))
	}
	for _, entry := range got {
		if entry.Category != domain.AuditCategorySystem {
			t.Errorf("category = %q, want the system category", entry.Category)
		}
		if !strings.Contains(entry.Input, "lt-1") || !strings.Contains(entry.Input, "pwsh") {
			t.Errorf("entry %q does not name the terminal and its shell", entry.Input)
		}
	}
	if !strings.Contains(got[0].Input, "opened") || !strings.Contains(got[1].Input, "closed") {
		t.Errorf("entries = %q / %q, want an opened then a closed", got[0].Input, got[1].Input)
	}
}

// TestAuditOffersNoWayToRecordKeystrokes guards the decision that a local shell's input is not
// logged. It is an absence, so it is asserted structurally: the recorder must expose exactly the
// two lifecycle methods, and adding a third that took user input would have to be written
// deliberately rather than arrived at.
func TestAuditOffersNoWayToRecordKeystrokes(t *testing.T) {
	var rec any = NewLocalTerminalAuditRecorder(&recordingAuditRepo{})

	if _, ok := rec.(interface{ Input(string, string) }); ok {
		t.Fatal("the recorder grew an Input method; a local shell's keystrokes must not be logged")
	}
	if _, ok := rec.(interface {
		RecordCommand(string, string)
	}); ok {
		t.Fatal("the recorder grew RecordCommand; that is the SSH session's contract, not this one")
	}
}

func TestAuditFailureDoesNotStopTheTerminal(t *testing.T) {
	// A sick audit database must not make the application unable to open a shell. The write is
	// attempted, the error is swallowed, and nothing panics.
	repo := &recordingAuditRepo{err: domain.ErrAuditLogWrite}
	rec := NewLocalTerminalAuditRecorder(repo)

	rec.Opened("lt-1", "bash")

	if len(repo.recorded()) != 1 {
		t.Error("the write was not even attempted")
	}
}

func TestNilRepoRecorderDropsEvents(t *testing.T) {
	rec := NewLocalTerminalAuditRecorder(nil)
	rec.Opened("lt-1", "bash")
	rec.Closed("lt-1", "bash")
}
