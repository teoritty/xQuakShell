package plugin_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

type memAuditRepo struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
}

func (r *memAuditRepo) Append(_ context.Context, entry domain.AuditEntry) error {
	r.mu.Lock()
	r.entries = append(r.entries, entry)
	r.mu.Unlock()
	return nil
}
func (r *memAuditRepo) Search(context.Context, string, domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	return nil, nil
}
func (r *memAuditRepo) DeleteByID(context.Context, int64) error         { return nil }
func (r *memAuditRepo) ClearAll(context.Context, string) error          { return nil }
func (r *memAuditRepo) Count(context.Context) (int64, error)            { return 0, nil }
func (r *memAuditRepo) PurgeOlderThan(context.Context, time.Time) error { return nil }
func (r *memAuditRepo) TrimToCount(context.Context, int) error          { return nil }
func (r *memAuditRepo) Close() error                                    { return nil }

func (r *memAuditRepo) lastInput() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return ""
	}
	return r.entries[len(r.entries)-1].Input
}

func TestPluginAuditWriter_OutboundAuthFunc(t *testing.T) {
	repo := &memAuditRepo{}
	writer := usecase.NewPluginAuditWriter(repo)
	fn := writer.OutboundAuthFunc()
	fn("com.test.auth", "auth.prepare", `{"fields":{"tenant":"acme"}}`)
	line := repo.lastInput()
	if !strings.Contains(line, "direction=outbound") {
		t.Fatalf("expected outbound audit line, got %q", line)
	}
	if !strings.Contains(line, "auth.prepare") {
		t.Fatalf("expected method in audit line, got %q", line)
	}
}
