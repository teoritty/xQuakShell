package auditlog

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"xquakshell/internal/domain"
)

// The PRAGMA cannot be parameterised, so the version appears twice. If someone bumps the constant
// and not the statement, every database stays stamped 1 and the migration reruns forever.
func TestAuditUserVersionStatementMatchesTheConstant(t *testing.T) {
	want := fmt.Sprintf("PRAGMA user_version = %d", auditSchemaVersion)
	if setAuditUserVersionSQL != want {
		t.Errorf("setAuditUserVersionSQL = %q, want %q", setAuditUserVersionSQL, want)
	}
}

// openLegacyAuditDB writes an audit.db in the shape the first release created: no category,
// connection_name, host, username or redacted column. This is what a database that has been
// carried across upgrades actually looks like, because CREATE TABLE IF NOT EXISTS never touched it.
func openLegacyAuditDB(t *testing.T, dir string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "audit.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	const legacyDDL = `
	CREATE TABLE audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		session_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		input TEXT NOT NULL
	);
	CREATE VIRTUAL TABLE audit_fts USING fts5(input, content='audit_events', content_rowid='id');
	CREATE TRIGGER audit_ai AFTER INSERT ON audit_events BEGIN
		INSERT INTO audit_fts(rowid, input) VALUES (new.id, new.input);
	END;
	INSERT INTO audit_events (ts, session_id, connection_id, input) VALUES ('2026-01-01T00:00:00Z', 's0', 'c0', 'legacy row');
	`
	if _, err := db.ExecContext(context.Background(), legacyDDL); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
}

func TestAuditSchemaMigrationRepairsADatabaseFromAnOlderBuild(t *testing.T) {
	dir := t.TempDir()
	openLegacyAuditDB(t, dir)

	repo, err := NewSQLiteRepo(dir)
	if err != nil {
		t.Fatalf("open repo over a legacy database: %v", err)
	}
	defer repo.db.Close()

	ctx := context.Background()

	present, err := auditEventsColumns(ctx, repo.db)
	if err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	for _, col := range auditEventsAddableColumns {
		if !present[col.name] {
			t.Errorf("column %q is still missing; every insert naming it fails at runtime", col.name)
		}
	}

	version, err := auditUserVersion(ctx, repo.db)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != auditSchemaVersion {
		t.Errorf("user_version = %d, want %d", version, auditSchemaVersion)
	}

	entry := domain.AuditEntry{
		Timestamp: time.Now(), Category: domain.AuditCategoryCommand,
		SessionID: "s1", ConnectionID: "c1", ConnectionName: "prod",
		Host: "example.com", Username: "root", Input: "whoami",
	}
	if err := repo.Append(ctx, entry); err != nil {
		t.Fatalf("append after migration: %v", err)
	}

	got, err := repo.Search(ctx, "whoami", domain.AuditSearchFilter{})
	if err != nil {
		t.Fatalf("search after migration: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("search returned %d rows, want 1", len(got))
	}
	if got[0].Host != "example.com" || got[0].ConnectionName != "prod" || got[0].Username != "root" {
		t.Errorf("back-filled columns did not round-trip: %+v", got[0])
	}
}

// Reopening must be a no-op. A migration that reruns on every start would keep issuing ALTER
// statements against a table that already has the columns and fail the second open.
func TestAuditSchemaMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	openLegacyAuditDB(t, dir)

	for attempt := 1; attempt <= 3; attempt++ {
		repo, err := NewSQLiteRepo(dir)
		if err != nil {
			t.Fatalf("open %d: %v", attempt, err)
		}
		repo.db.Close()
	}
}

// A fresh database is created complete by the DDL, so the migration has nothing to add and only
// stamps the version.
func TestAuditSchemaVersionIsStampedOnAFreshDatabase(t *testing.T) {
	repo, err := NewSQLiteRepo(t.TempDir())
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	defer repo.db.Close()

	version, err := auditUserVersion(context.Background(), repo.db)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != auditSchemaVersion {
		t.Errorf("user_version = %d, want %d", version, auditSchemaVersion)
	}
}
