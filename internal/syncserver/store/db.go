// Package store is the sync server's SQLite adapter (ADR-023), implementing usecase.Store. It holds
// ciphertext, public keys and signed logs; nothing in it opens a vault. Secure delete is on, so a
// row the server deletes is overwritten in the file rather than left for a disk image to recover.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"xquakshell/internal/syncserver/usecase"
)

// DB is an open server database.
type DB struct {
	db *sql.DB
	// writes orders this process's writers before they reach SQLite; see writeQueue.
	writes writeQueue
}

var _ usecase.Store = (*DB)(nil)

// Open opens or creates the database at path. A database written by a newer build is refused
// before anything writes to it, and so is a database this program did not create.
func Open(ctx context.Context, path string) (*DB, error) {
	if err := checkExisting(ctx, path); err != nil {
		return nil, err
	}
	q := url.Values{}
	for _, p := range []string{"journal_mode(wal)", "foreign_keys(1)", "secure_delete(1)", "busy_timeout(5000)"} {
		q.Add("_pragma", p)
	}
	// Every use case transaction reads before it writes. A deferred transaction reads under a
	// snapshot, and when another connection commits first, SQLite refuses the upgrade to a writer at
	// once with SQLITE_BUSY - the busy timeout cannot help, since no wait makes the snapshot current.
	// Taking the write lock at BEGIN makes a second writer wait its turn instead. The cost is that
	// transactions run one at a time, which a server holding a few teams' vaults never notices. The
	// turn itself is given out by writeQueue, in order, before BEGIN is sent.
	q.Add("_txlock", "immediate")
	db, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	s := &DB{db: db}
	if err := s.ensureSchema(ctx, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// checkExisting reads the schema version through a read-only connection with no pragmas, because
// even switching the journal mode rewrites the file header - and a newer server's database must be
// left byte for byte as it was.
func checkExisting(ctx context.Context, path string) error {
	ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("store: open read-only: %w", err)
	}
	defer func() { _ = ro.Close() }()
	version, tables, err := inspect(ctx, ro)
	if err != nil {
		// No file yet: mode=ro cannot create one, and there is nothing to protect.
		return nil
	}
	switch {
	case version > schemaVersion:
		return fmt.Errorf("store: database schema %d, this build reads %d: %w", version, schemaVersion, ErrStoreTooNew)
	case version == 0 && tables > 0:
		return ErrStoreUnknown
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

// Close releases the connection pool; the WAL is folded back on the last connection closing.
func (s *DB) Close() error { return s.db.Close() }

// WithTx runs fn in one transaction and commits only if fn returns nil. It waits for its turn in
// the write queue first, so fn must not start another transaction: it would wait behind itself.
func (s *DB) WithTx(ctx context.Context, fn func(usecase.Tx) error) error {
	if err := s.writes.acquire(ctx); err != nil {
		return fmt.Errorf("store: wait to write: %w", err)
	}
	defer s.writes.release()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	if err := fn(&txn{ctx: ctx, tx: tx}); err != nil {
		if rb := tx.Rollback(); rb != nil && !errors.Is(rb, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("store: rollback: %w", rb))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

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
// It takes a turn in the write queue like a transaction does: a truncating checkpoint waits for
// every writer, and in SQLite's own wait it would starve behind them the same way.
func (s *DB) Checkpoint(ctx context.Context) error {
	if err := s.writes.acquire(ctx); err != nil {
		return fmt.Errorf("store: wait to checkpoint: %w", err)
	}
	defer s.writes.release()
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// txn is one transaction; every query method of usecase.Tx hangs off it.
type txn struct {
	ctx context.Context
	tx  *sql.Tx
}

func (t *txn) exec(query string, args ...any) error {
	_, err := t.tx.ExecContext(t.ctx, query, args...)
	return err
}

func (t *txn) changed(query string, args ...any) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// found turns sql.ErrNoRows into a clean "not found".
func found(err error) (bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
