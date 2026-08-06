package auditlog

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"xquakshell/internal/domain"
)

// SQLiteRepo implements domain.AuditLogRepository using SQLite with FTS5.
type SQLiteRepo struct {
	db *sql.DB
}

// NewSQLiteRepo creates and initializes an audit log SQLite database in the given directory.
func NewSQLiteRepo(dir string) (*SQLiteRepo, error) {
	dbPath := filepath.Join(dir, "audit.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)")
	if err != nil {
		return nil, fmt.Errorf("audit open db: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepo{db: db}, nil
}

func initSchema(db *sql.DB) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		category TEXT NOT NULL DEFAULT 'command',
		session_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		connection_name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '',
		input TEXT NOT NULL,
		redacted INTEGER NOT NULL DEFAULT 0
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS audit_fts USING fts5(
		input,
		content='audit_events',
		content_rowid='id'
	);

	CREATE TRIGGER IF NOT EXISTS audit_ai AFTER INSERT ON audit_events BEGIN
		INSERT INTO audit_fts(rowid, input) VALUES (new.id, new.input);
	END;

	CREATE TRIGGER IF NOT EXISTS audit_ad AFTER DELETE ON audit_events BEGIN
		INSERT INTO audit_fts(audit_fts, rowid, input) VALUES ('delete', old.id, old.input);
	END;

	CREATE INDEX IF NOT EXISTS idx_audit_session ON audit_events(session_id);
	CREATE INDEX IF NOT EXISTS idx_audit_connection ON audit_events(connection_id);
	CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_events(ts);
	`
	// Schema creation runs inside NewSQLiteRepo, which has no caller context to
	// inherit: an audit database that failed to open half-way is worse than one
	// that took a moment longer, so this one is not cancellable by design.
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		return fmt.Errorf("audit init schema: %w", err)
	}
	return nil
}

// Append writes a new audit entry to the log.
func (r *SQLiteRepo) Append(ctx context.Context, entry domain.AuditEntry) error {
	ts := entry.Timestamp.UTC().Format(time.RFC3339Nano)
	redacted := 0
	if entry.Redacted {
		redacted = 1
	}
	category := entry.Category
	if category == "" {
		category = domain.AuditCategoryCommand
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO audit_events (ts, category, session_id, connection_id, connection_name, host, username, input, redacted)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, category, entry.SessionID, entry.ConnectionID, entry.ConnectionName, entry.Host, entry.Username, entry.Input, redacted,
	)
	if err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}

// Search performs full-text search on audit entries with optional filters.
func (r *SQLiteRepo) Search(ctx context.Context, query string, filter domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	var args []interface{}
	var whereClauses []string

	baseQuery := `SELECT e.id, e.ts, e.category, e.session_id, e.connection_id, e.connection_name, e.host, e.username, e.input, e.redacted
		FROM audit_events e`

	if query != "" {
		baseQuery += ` INNER JOIN audit_fts f ON f.rowid = e.id`
		whereClauses = append(whereClauses, `audit_fts MATCH ?`)
		args = append(args, query)
	}

	if filter.Category != "" {
		whereClauses = append(whereClauses, `e.category = ?`)
		args = append(args, filter.Category)
	}
	if filter.SessionID != "" {
		whereClauses = append(whereClauses, `e.session_id = ?`)
		args = append(args, filter.SessionID)
	}
	if filter.ConnectionID != "" {
		whereClauses = append(whereClauses, `e.connection_id = ?`)
		args = append(args, filter.ConnectionID)
	}
	if filter.From != nil {
		whereClauses = append(whereClauses, `e.ts >= ?`)
		args = append(args, filter.From.UTC().Format(time.RFC3339Nano))
	}
	if filter.To != nil {
		whereClauses = append(whereClauses, `e.ts <= ?`)
		args = append(args, filter.To.UTC().Format(time.RFC3339Nano))
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE "
		for i, clause := range whereClauses {
			if i > 0 {
				baseQuery += " AND "
			}
			baseQuery += clause
		}
	}

	baseQuery += ` ORDER BY e.ts DESC`

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	// #nosec G202 -- LIMIT/OFFSET cannot be bound as parameters on every driver, and
	// %d on an int renders digits only, so no operand here can carry SQL. Every value
	// that originates from a user string goes through args and the placeholders above.
	baseQuery += fmt.Sprintf(` LIMIT %d`, limit)
	if filter.Offset > 0 {
		// #nosec G202 -- same as LIMIT above: %d on an int cannot carry SQL.
		baseQuery += fmt.Sprintf(` OFFSET %d`, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("audit search: %w", err)
	}
	defer rows.Close()

	var results []domain.AuditEntry
	for rows.Next() {
		var e domain.AuditEntry
		var tsStr string
		var redacted int
		if err := rows.Scan(&e.ID, &tsStr, &e.Category, &e.SessionID, &e.ConnectionID, &e.ConnectionName, &e.Host, &e.Username, &e.Input, &redacted); err != nil {
			return nil, fmt.Errorf("audit scan: %w", err)
		}
		ts, parseErr := time.Parse(time.RFC3339Nano, tsStr)
		if parseErr != nil {
			slog.Warn("audit: failed to parse timestamp", "raw", tsStr, "err", parseErr)
			ts = time.Now()
		}
		e.Timestamp = ts
		e.Redacted = redacted != 0
		results = append(results, e)
	}
	return results, rows.Err()
}

// DeleteByID removes a single audit entry by ID.
func (r *SQLiteRepo) DeleteByID(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM audit_events WHERE id = ?`, id)
	return err
}

// ClearAll removes audit entries. An empty category clears all entries;
// a non-empty category clears only entries of that category.
func (r *SQLiteRepo) ClearAll(ctx context.Context, category string) error {
	if category == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM audit_events`)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM audit_events WHERE category = ?`, category)
	return err
}

// Count returns the total number of audit entries.
func (r *SQLiteRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&n)
	return n, err
}

// PurgeOlderThan deletes audit entries older than cutoff.
func (r *SQLiteRepo) PurgeOlderThan(ctx context.Context, cutoff time.Time) error {
	cutoffStr := cutoff.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `DELETE FROM audit_events WHERE ts < ?`, cutoffStr)
	return err
}

// TrimToCount deletes oldest entries until at most max remain.
func (r *SQLiteRepo) TrimToCount(ctx context.Context, max int) error {
	if max <= 0 {
		return nil
	}
	count, err := r.Count(ctx)
	if err != nil {
		return err
	}
	excess := int(count) - max
	if excess <= 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `
		DELETE FROM audit_events WHERE id IN (
			SELECT id FROM audit_events ORDER BY ts ASC LIMIT ?
		)`, excess)
	return err
}

// Close releases the database connection.
func (r *SQLiteRepo) Close() error {
	return r.db.Close()
}

// PurgeOlderThanNow deletes audit entries older than the given duration from now.
//
// The context-free signature is the port's, so the context has to be minted
// here. Retention is a policy the vault owner set, not a request anyone is
// waiting on, and a purge that gets cancelled leaves data past its retention
// window - which is the failure this exists to prevent.
func (r *SQLiteRepo) PurgeOlderThanNow(d time.Duration) error {
	return r.PurgeOlderThan(context.Background(), time.Now().Add(-d))
}
