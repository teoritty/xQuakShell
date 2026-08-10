package auditlog

import (
	"context"
	"database/sql"
	"fmt"
)

// auditSchemaVersion is the audit_events shape this build expects, recorded in the database as
// PRAGMA user_version. Version 0 means a database written before the schema carried a version at
// all — every audit.db that existed before this was added.
const auditSchemaVersion = 1

// setAuditUserVersionSQL stamps the version. PRAGMA does not accept a bound parameter and the
// value is a constant, so it is spelled out rather than formatted in: a Sprintf here would read
// like a query built from a variable at every future audit of this file. A test holds the literal
// against auditSchemaVersion so the two cannot drift.
const setAuditUserVersionSQL = "PRAGMA user_version = 1"

// auditEventsRequiredColumns are the columns SQLite cannot add after the fact: the autoincrement
// primary key, and the NOT NULL columns with no default. If one of these is missing the database
// was not written by this application and there is nothing safe to repair.
var auditEventsRequiredColumns = []string{"id", "ts", "session_id", "connection_id", "input"}

// auditEventsAddableColumns are the columns added to audit_events after its first release, each
// with the statement that back-fills it. CREATE TABLE IF NOT EXISTS silently skips a table that
// already exists, so a database created by an earlier build kept the old column set forever and
// every insert naming these columns failed at runtime. They all carry a default, which is what
// makes ADD COLUMN legal on a NOT NULL column.
var auditEventsAddableColumns = []struct {
	name string
	add  string
}{
	{"category", "ALTER TABLE audit_events ADD COLUMN category TEXT NOT NULL DEFAULT 'command'"},
	{"connection_name", "ALTER TABLE audit_events ADD COLUMN connection_name TEXT NOT NULL DEFAULT ''"},
	{"host", "ALTER TABLE audit_events ADD COLUMN host TEXT NOT NULL DEFAULT ''"},
	{"username", "ALTER TABLE audit_events ADD COLUMN username TEXT NOT NULL DEFAULT ''"},
	{"redacted", "ALTER TABLE audit_events ADD COLUMN redacted INTEGER NOT NULL DEFAULT 0"},
}

// migrateAuditSchema brings an existing audit_events table up to auditSchemaVersion.
//
// It reconciles against the live table rather than replaying a list of "what changed since
// release X", because the databases in the wild were never stamped with a release to begin with —
// asking the table what it has is the only question that has a reliable answer. The full-text
// index is untouched: audit_fts has only ever indexed the input column, so none of the added
// columns affect it.
func migrateAuditSchema(ctx context.Context, db *sql.DB) error {
	version, err := auditUserVersion(ctx, db)
	if err != nil {
		return err
	}
	if version >= auditSchemaVersion {
		return nil
	}

	present, err := auditEventsColumns(ctx, db)
	if err != nil {
		return err
	}
	for _, name := range auditEventsRequiredColumns {
		if !present[name] {
			return fmt.Errorf("audit migrate: audit_events has no %q column and it cannot be added", name)
		}
	}
	for _, col := range auditEventsAddableColumns {
		if present[col.name] {
			continue
		}
		if _, err := db.ExecContext(ctx, col.add); err != nil {
			return fmt.Errorf("audit migrate: add column %s: %w", col.name, err)
		}
	}

	if _, err := db.ExecContext(ctx, setAuditUserVersionSQL); err != nil {
		return fmt.Errorf("audit migrate: stamp schema version: %w", err)
	}
	return nil
}

func auditUserVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("audit migrate: read schema version: %w", err)
	}
	return version, nil
}

// auditEventsColumns reports which columns the table actually has. table_info returns six columns
// per row and Scan needs a destination for each, so the unused ones are read into throwaways.
func auditEventsColumns(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(audit_events)")
	if err != nil {
		return nil, fmt.Errorf("audit migrate: inspect audit_events: %w", err)
	}
	defer rows.Close()

	present := make(map[string]bool)
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, declType   string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &declType, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("audit migrate: scan audit_events column: %w", err)
		}
		present[name] = true
	}
	return present, rows.Err()
}
