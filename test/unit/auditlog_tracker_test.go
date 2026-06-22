package unit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
)

func TestCommandLineTrackerHtopAfterBackspace(t *testing.T) {
	tracker := auditlog.NewCommandLineTracker()
	for _, ch := range "ls\x7f\x7f\x7f\x7fhtop" {
		_, ok := tracker.Feed(string(ch))
		if ok {
			t.Fatalf("unexpected submit after %q", string(ch))
		}
	}
	line, ok := tracker.Feed("\r")
	if !ok {
		t.Fatal("expected submit on enter")
	}
	if line != "htop" {
		t.Errorf("want htop, got %q", line)
	}
}

func TestCommandLineTrackerClearWithoutCtrlC(t *testing.T) {
	tracker := auditlog.NewCommandLineTracker()
	for _, ch := range "clear" {
		_, ok := tracker.Feed(string(ch))
		if ok {
			t.Fatal("unexpected early submit")
		}
	}
	line, ok := tracker.Feed("\n")
	if !ok {
		t.Fatal("expected submit")
	}
	if line != "clear" {
		t.Errorf("want clear, got %q", line)
	}
}

func TestCommandLineTrackerHomeInsert(t *testing.T) {
	tracker := auditlog.NewCommandLineTracker()
	for _, ch := range "apt update" {
		tracker.Feed(string(ch))
	}
	tracker.Feed("\x1b[H")
	for _, ch := range "sudo " {
		tracker.Feed(string(ch))
	}
	line, ok := tracker.Feed("\r")
	if !ok {
		t.Fatal("expected submit")
	}
	if line != "sudo apt update" {
		t.Errorf("want %q, got %q", "sudo apt update", line)
	}
}

func TestCommandLineTrackerArrowOnlyNoSubmit(t *testing.T) {
	tracker := auditlog.NewCommandLineTracker()
	seqs := []string{"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F"}
	for _, seq := range seqs {
		line, ok := tracker.Feed(seq)
		if ok {
			t.Fatalf("arrow-only input should not submit, got %q", line)
		}
	}
}

func TestAuditLogRetentionTrimToCount(t *testing.T) {
	dir := t.TempDir()
	repo, err := auditlog.NewSQLiteRepo(dir)
	if err != nil {
		t.Fatalf("NewSQLiteRepo: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	for i := 0; i < 6; i++ {
		entry := domain.AuditEntry{
			Timestamp:    time.Now().Add(time.Duration(i) * time.Second),
			SessionID:    "s1",
			ConnectionID: "c1",
			Input:        fmt.Sprintf("cmd%d", i),
		}
		if err := repo.Append(ctx, entry); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := repo.TrimToCount(ctx, 5); err != nil {
		t.Fatalf("TrimToCount: %v", err)
	}
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 5 {
		t.Errorf("want 5 entries, got %d", count)
	}
}
