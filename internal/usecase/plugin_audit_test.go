package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
)

type auditRepoStub struct {
	last domain.AuditEntry
}

func (a *auditRepoStub) Append(_ context.Context, entry domain.AuditEntry) error {
	a.last = entry
	return nil
}
func (a *auditRepoStub) Search(context.Context, string, domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	return nil, nil
}
func (a *auditRepoStub) DeleteByID(context.Context, int64) error { return nil }
func (a *auditRepoStub) ClearAll(context.Context) error          { return nil }
func (a *auditRepoStub) Count(context.Context) (int64, error)    { return 0, nil }
func (a *auditRepoStub) PurgeOlderThan(context.Context, time.Time) error {
	return nil
}
func (a *auditRepoStub) TrimToCount(context.Context, int) error { return nil }
func (a *auditRepoStub) Close() error                           { return nil }

func TestPluginAuditWriterRedactsSecrets(t *testing.T) {
	repo := &auditRepoStub{}
	writer := usecase.NewPluginAuditWriter(repo)
	writer.RPCRecorder()("com.test", "vault.getSecret", true, "password=secret123")

	if strings.Contains(repo.last.Input, "secret123") {
		t.Fatalf("audit line leaked secret: %q", repo.last.Input)
	}
	if !strings.Contains(repo.last.Input, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", repo.last.Input)
	}
}
