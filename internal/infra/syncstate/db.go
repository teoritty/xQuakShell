// Package syncstate is the device's sync state database, data/sync/state.db (ADR-023, plan P4-3).
// It holds server envelopes, signed logs and the outbox - ciphertext and public data only. The
// plaintext of every record lives in the encrypted vault and nowhere else. Secure delete is on, so a
// forgotten vault's ciphertext is overwritten in the file rather than left for a disk image.
package syncstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"xquakshell/internal/domain/synctrust"
)

// schemaVersion is the state.db shape this build reads and writes (CHANGELOG on-disk schema axis).
const schemaVersion = 1

// setSchemaVersionSQL stamps the version; PRAGMA takes no bound parameter, so it is a literal.
const setSchemaVersionSQL = "PRAGMA user_version = 1"

var (
	// ErrStateTooNew is a state file written by a newer build. It is never opened for writing:
	// writing back only the columns this build knows would drop the rest.
	ErrStateTooNew = errors.New("sync state written by a newer xQuakShell")
	// ErrStateUnknown is a non-empty database without a schema version - not one of ours.
	ErrStateUnknown = errors.New("sync state file was not created by xQuakShell")
)

const schemaV1 = `
CREATE TABLE heads (vault BLOB PRIMARY KEY, seq INTEGER NOT NULL, head BLOB NOT NULL);
CREATE TABLE records (vault BLOB NOT NULL, record BLOB NOT NULL, envelope BLOB NOT NULL,
  PRIMARY KEY (vault, record));
CREATE TABLE device_entries (account BLOB NOT NULL, seq INTEGER NOT NULL, entry BLOB NOT NULL,
  PRIMARY KEY (account, seq));
CREATE TABLE member_ops (vault BLOB NOT NULL, seq INTEGER NOT NULL, op BLOB NOT NULL,
  PRIMARY KEY (vault, seq));
CREATE TABLE outbox (vault BLOB PRIMARY KEY, commit_id BLOB NOT NULL, payload BLOB NOT NULL);
CREATE TABLE quarantine (vault BLOB NOT NULL, record BLOB NOT NULL, envelope BLOB NOT NULL,
  reason TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY (vault, record));
`

// DB is an open state database.
type DB struct {
	db *sql.DB
}

var _ synctrust.StateStore = (*DB)(nil)

// Open opens or creates the state database at path. A file written by a newer build is refused
// before anything writes to it, and so is a database this program did not create.
func Open(ctx context.Context, path string) (*DB, error) {
	if err := checkExisting(ctx, path); err != nil {
		return nil, err
	}
	q := url.Values{}
	for _, p := range []string{"journal_mode(wal)", "secure_delete(1)", "busy_timeout(5000)"} {
		q.Add("_pragma", p)
	}
	db, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("syncstate: open: %w", err)
	}
	s := &DB{db: db}
	if err := s.ensureSchema(ctx); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, errors.Join(err, cerr)
		}
		return nil, err
	}
	return s, nil
}

// checkExisting reads the schema version through a read-only connection with no pragmas, because
// even switching the journal mode rewrites the file header - and a newer build's file must be left
// byte for byte as it was.
func checkExisting(ctx context.Context, path string) error {
	ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("syncstate: open read-only: %w", err)
	}
	defer func() { _ = ro.Close() }()
	version, tables, err := inspect(ctx, ro)
	if err != nil {
		// No file yet: mode=ro cannot create one, and there is nothing to protect.
		return nil
	}
	switch {
	case version > schemaVersion:
		return fmt.Errorf("syncstate: schema %d, this build reads %d: %w", version, schemaVersion, ErrStateTooNew)
	case version == 0 && tables > 0:
		return ErrStateUnknown
	}
	return nil
}

func inspect(ctx context.Context, db *sql.DB) (version, tables int, err error) {
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, 0, err
	}
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables)
	return version, tables, err
}

func (s *DB) ensureSchema(ctx context.Context) error {
	version, tables, err := inspect(ctx, s.db)
	if err != nil {
		return fmt.Errorf("syncstate: inspect: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 || tables != 0 {
		return ErrStateUnknown
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("syncstate: create schema: %w", err)
		}
		if _, err := tx.ExecContext(ctx, setSchemaVersionSQL); err != nil {
			return fmt.Errorf("syncstate: stamp schema: %w", err)
		}
		return nil
	})
}

// withTx runs fn in one transaction and commits only if fn returns nil.
func (s *DB) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("syncstate: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rb := tx.Rollback(); rb != nil && !errors.Is(rb, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("syncstate: rollback: %w", rb))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("syncstate: commit: %w", err)
	}
	return nil
}

// Close releases the connection pool; the WAL is folded back on the last connection closing.
func (s *DB) Close() error { return s.db.Close() }

// SchemaVersion reports the database's PRAGMA user_version.
func (s *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

// SecureDelete reports whether deleted content is overwritten.
func (s *DB) SecureDelete(ctx context.Context) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA secure_delete").Scan(&v)
	return v == 1, err
}

// Checkpoint folds the write-ahead log into the main file and truncates it, so deleted content
// that only the log still held is gone too.
func (s *DB) Checkpoint(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
