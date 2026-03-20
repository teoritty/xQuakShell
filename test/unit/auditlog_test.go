package unit_test

import (
	"context"
	"testing"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
)

func TestAuditLogAppendAndSearch(t *testing.T) {
	dir := t.TempDir()
	repo, err := auditlog.NewSQLiteRepo(dir)
	if err != nil {
		t.Fatalf("NewSQLiteRepo: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	now := time.Now()
	sessionID := "sess-1"
	connID := "conn-1"

	entries := []domain.AuditEntry{
		{Timestamp: now, SessionID: sessionID, ConnectionID: connID, Input: "ls -la"},
		{Timestamp: now.Add(time.Second), SessionID: sessionID, ConnectionID: connID, Input: "cat /etc/passwd"},
		{Timestamp: now.Add(2 * time.Second), SessionID: sessionID, ConnectionID: connID, Input: "whoami"},
	}

	for _, e := range entries {
		if err := repo.Append(ctx, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Empty query → all 3 entries
	all, err := repo.Search(ctx, "", domain.AuditSearchFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Search empty: want 3 entries, got %d", len(all))
	}

	// Query "passwd" → 1 entry
	passwd, err := repo.Search(ctx, "passwd", domain.AuditSearchFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Search passwd: %v", err)
	}
	if len(passwd) != 1 {
		t.Errorf("Search passwd: want 1 entry, got %d", len(passwd))
	}
	if len(passwd) > 0 && passwd[0].Input != "cat /etc/passwd" {
		t.Errorf("Search passwd: got input %q", passwd[0].Input)
	}
}

func TestAuditLogSearchBySession(t *testing.T) {
	dir := t.TempDir()
	repo, err := auditlog.NewSQLiteRepo(dir)
	if err != nil {
		t.Fatalf("NewSQLiteRepo: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	now := time.Now()

	entries := []domain.AuditEntry{
		{Timestamp: now, SessionID: "sess-A", ConnectionID: "c1", Input: "cmd1"},
		{Timestamp: now.Add(time.Second), SessionID: "sess-B", ConnectionID: "c2", Input: "cmd2"},
		{Timestamp: now.Add(2 * time.Second), SessionID: "sess-A", ConnectionID: "c1", Input: "cmd3"},
	}

	for _, e := range entries {
		if err := repo.Append(ctx, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	filter := domain.AuditSearchFilter{SessionID: "sess-A", Limit: 100}
	results, err := repo.Search(ctx, "", filter)
	if err != nil {
		t.Fatalf("Search by session: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Search sess-A: want 2 entries, got %d", len(results))
	}
	for _, r := range results {
		if r.SessionID != "sess-A" {
			t.Errorf("unexpected session %q", r.SessionID)
		}
	}
}

func TestSanitizerMasksPassword(t *testing.T) {
	s := auditlog.NewSanitizer()

	// Feed password prompt output
	s.FeedOutput("Enter password: ")

	// Sanitize password input → redacted
	out, redacted := s.SanitizeInput("my-secret\n")
	if !redacted {
		t.Error("expected redacted=true for password input")
	}
	if out != "***REDACTED***" {
		t.Errorf("expected ***REDACTED***, got %q", out)
	}

	// Feed normal output (no password prompt)
	s.FeedOutput("$ ")

	// Sanitize normal input → not redacted
	out, redacted = s.SanitizeInput("ls\n")
	if redacted {
		t.Error("expected redacted=false for normal input")
	}
	if out != "ls\n" {
		t.Errorf("expected ls\\n, got %q", out)
	}
}

func TestSanitizerNormalInput(t *testing.T) {
	s := auditlog.NewSanitizer()
	s.FeedOutput("$ ")

	out, redacted := s.SanitizeInput("ls")
	if redacted {
		t.Error("expected redacted=false for normal input")
	}
	if out != "ls" {
		t.Errorf("expected ls, got %q", out)
	}
}
