package usecase

import (
	"context"
	"log/slog"

	"xquakshell/internal/domain"
)

// LocalTerminalAuditRecorder writes local terminal starts and stops to the audit log.
//
// Two methods, and the ones that are absent matter as much as the ones that are here. An SSH
// session reconstructs and records every command line the user submits, because the audit trail
// is about what was done to somebody else's machine. A local terminal is the user's own computer,
// where they already have a shell and a shell history; recording their keystrokes would collect
// passwords typed into command arguments in exchange for telling nobody anything they could not
// get from their own history. What is worth recording is that the application started a shell at
// all, and which one - the security-relevant fact is the capability being used, not its content.
type LocalTerminalAuditRecorder struct {
	repo domain.AuditLogRepository
}

// NewLocalTerminalAuditRecorder returns a recorder writing to repo. A nil repo yields one that
// drops events, so a build whose audit database failed to open can still open a terminal.
func NewLocalTerminalAuditRecorder(repo domain.AuditLogRepository) *LocalTerminalAuditRecorder {
	return &LocalTerminalAuditRecorder{repo: repo}
}

var _ LocalTerminalAuditor = (*LocalTerminalAuditRecorder)(nil)

// Opened records that a shell was started.
func (r *LocalTerminalAuditRecorder) Opened(id, shellName string) {
	r.record("local terminal opened", id, shellName)
}

// Closed records that a shell ended, whether the user closed the tab or the shell exited.
func (r *LocalTerminalAuditRecorder) Closed(id, shellName string) {
	r.record("local terminal closed", id, shellName)
}

func (r *LocalTerminalAuditRecorder) record(event, id, shellName string) {
	if r == nil || r.repo == nil {
		return
	}
	entry := domain.AuditEntry{
		Category: domain.AuditCategorySystem,
		Input:    event + " id=" + id + " shell=" + shellName,
	}
	// A detached context: this describes something that already happened, and the close half runs
	// while the application is shutting down, when the context that would otherwise be passed in
	// is exactly the one being cancelled. Threading that through would drop the record of every
	// terminal closed at exit.
	if err := r.repo.Append(context.Background(), entry); err != nil {
		// A failed audit write must not fail the terminal it describes; logged at warn so the gap
		// in the trail is visible rather than silent.
		slog.Warn("local terminal audit write failed", "event", event, "id", id, "err", err)
	}
}
