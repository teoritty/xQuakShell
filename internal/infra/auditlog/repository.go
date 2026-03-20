package auditlog

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"ssh-client/internal/domain"
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
		session_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
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
	_, err := db.Exec(ddl)
	if err != nil {
		return fmt.Errorf("audit init schema: %w", err)
	}
	return nil
}

// Append writes a new audit entry to the log.
func (r *SQLiteRepo) Append(_ context.Context, entry domain.AuditEntry) error {
	ts := entry.Timestamp.UTC().Format(time.RFC3339Nano)
	redacted := 0
	if entry.Redacted {
		redacted = 1
	}
	_, err := r.db.Exec(
		`INSERT INTO audit_events (ts, session_id, connection_id, username, input, redacted)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ts, entry.SessionID, entry.ConnectionID, entry.Username, entry.Input, redacted,
	)
	if err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}

// Search performs full-text search on audit entries with optional filters.
func (r *SQLiteRepo) Search(_ context.Context, query string, filter domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	var args []interface{}
	var whereClauses []string

	baseQuery := `SELECT e.id, e.ts, e.session_id, e.connection_id, e.username, e.input, e.redacted
		FROM audit_events e`

	if query != "" {
		baseQuery += ` INNER JOIN audit_fts f ON f.rowid = e.id`
		whereClauses = append(whereClauses, `audit_fts MATCH ?`)
		args = append(args, query)
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
	baseQuery += fmt.Sprintf(` LIMIT %d`, limit)
	if filter.Offset > 0 {
		baseQuery += fmt.Sprintf(` OFFSET %d`, filter.Offset)
	}

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("audit search: %w", err)
	}
	defer rows.Close()

	var results []domain.AuditEntry
	for rows.Next() {
		var e domain.AuditEntry
		var tsStr string
		var redacted int
		if err := rows.Scan(&e.ID, &tsStr, &e.SessionID, &e.ConnectionID, &e.Username, &e.Input, &redacted); err != nil {
			return nil, fmt.Errorf("audit scan: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
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

// ClearAll removes all audit entries.
func (r *SQLiteRepo) ClearAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM audit_events`)
	return err
}

// PurgeOlderThan deletes audit entries older than the given duration.
func (r *SQLiteRepo) PurgeOlderThan(d time.Duration) error {
	cutoff := time.Now().Add(-d).UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(`DELETE FROM audit_events WHERE ts < ?`, cutoff)
	return err
}

// Close releases the database connection.
func (r *SQLiteRepo) Close() error {
	r.PurgeOlderThan(30 * 24 * time.Hour)
	return r.db.Close()
}
