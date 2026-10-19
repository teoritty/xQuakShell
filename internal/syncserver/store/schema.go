package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

// schemaVersion is the server database shape this build reads and writes (ADR-023 server store
// schema axis). Any bump needs a migration that backs the file up first, as ADR-017 §4 requires.
const schemaVersion = 2

// setSchemaVersionSQL stamps the version; PRAGMA takes no bound parameter, so it is a literal.
const setSchemaVersionSQL = "PRAGMA user_version = 2"

var (
	// ErrStoreTooNew is a database written by a newer server. It is never opened for writing:
	// writing back only the columns this build knows would drop the rest.
	ErrStoreTooNew = errors.New("store: database written by a newer xqsyncd")
	// ErrStoreUnknown is a non-empty database without a schema version - not one of ours.
	ErrStoreUnknown = errors.New("store: database was not created by xqsyncd")
)

// record_versions keeps, beside each envelope, the two things a Merkle leaf needs from it: the
// envelope digest and whether it is a tombstone. Verifying a commit rebuilds the vault's state from
// these alone, rather than reading and hashing every ciphertext the vault holds on every commit.
const schemaV2 = `
CREATE TABLE accounts (id BLOB PRIMARY KEY, username TEXT NOT NULL UNIQUE, opaque_record BLOB NOT NULL,
  bundle BLOB NOT NULL, identity_key BLOB NOT NULL, encryption_key BLOB NOT NULL,
  created INTEGER NOT NULL, frozen INTEGER NOT NULL DEFAULT 0);
CREATE TABLE invites (code_hash BLOB PRIMARY KEY, expires INTEGER NOT NULL, used_by BLOB);
CREATE TABLE device_entries (account BLOB NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL, entry BLOB NOT NULL, PRIMARY KEY (account, seq));
CREATE TABLE devices (id BLOB PRIMARY KEY, account BLOB NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  signing_key BLOB NOT NULL, revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE tokens (hash BLOB PRIMARY KEY, account BLOB NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device BLOB, kind TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE vaults (id BLOB PRIMARY KEY, owner BLOB NOT NULL, seq INTEGER NOT NULL DEFAULT 0,
  membership_seq INTEGER NOT NULL DEFAULT 0, bytes INTEGER NOT NULL DEFAULT 0,
  personal INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0, pruned_seq INTEGER NOT NULL DEFAULT 0);
CREATE TABLE member_ops (vault BLOB NOT NULL REFERENCES vaults(id) ON DELETE CASCADE, seq INTEGER NOT NULL,
  op BLOB NOT NULL, PRIMARY KEY (vault, seq));
CREATE TABLE members (vault BLOB NOT NULL REFERENCES vaults(id) ON DELETE CASCADE, account BLOB NOT NULL,
  role INTEGER NOT NULL, PRIMARY KEY (vault, account));
CREATE INDEX members_account ON members (account);
CREATE TABLE participants (vault BLOB NOT NULL REFERENCES vaults(id) ON DELETE CASCADE, account BLOB NOT NULL,
  PRIMARY KEY (vault, account));
CREATE TABLE commits (vault BLOB NOT NULL REFERENCES vaults(id) ON DELETE CASCADE, seq INTEGER NOT NULL,
  commit_id BLOB NOT NULL, head BLOB NOT NULL, purged BLOB NOT NULL, received INTEGER NOT NULL,
  PRIMARY KEY (vault, seq), UNIQUE (vault, commit_id));
CREATE TABLE record_versions (vault BLOB NOT NULL REFERENCES vaults(id) ON DELETE CASCADE, record BLOB NOT NULL,
  version INTEGER NOT NULL, seq INTEGER NOT NULL, epoch INTEGER NOT NULL, size INTEGER NOT NULL,
  envelope BLOB NOT NULL, digest BLOB NOT NULL, deleted INTEGER NOT NULL, PRIMARY KEY (vault, record, version));
CREATE INDEX record_versions_seq ON record_versions (vault, seq);
`

// migrateV1Columns adds the leaf columns to a schema 1 table; migrateV1 fills them in.
const migrateV1Columns = `
ALTER TABLE record_versions ADD COLUMN digest BLOB NOT NULL DEFAULT x'';
ALTER TABLE record_versions ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0;
`

// ensureSchema creates the schema in a fresh database and migrates one from schema 1. An existing
// one at this version needs nothing; checkExisting has already refused every other case.
func (s *DB) ensureSchema(ctx context.Context, path string) error {
	version, tables, err := inspect(ctx, s.db)
	if err != nil {
		return fmt.Errorf("store: inspect: %w", err)
	}
	switch {
	case version == schemaVersion:
		return nil
	case version == 1:
		return s.migrateFromV1(ctx, path)
	case version != 0 || tables != 0:
		return ErrStoreUnknown
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
			return fmt.Errorf("store: create schema: %w", err)
		}
		return stampSchema(ctx, tx)
	})
}

func stampSchema(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, setSchemaVersionSQL); err != nil {
		return fmt.Errorf("store: stamp schema: %w", err)
	}
	return nil
}

// inTx runs fn in one transaction outside the write queue: only Open uses it, before the database
// is shared with anything that queues.
func (s *DB) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin schema: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// BackupPath names the copy a migration takes of a database at fromVersion before it changes it.
func BackupPath(path string, fromVersion int) string {
	return fmt.Sprintf("%s.v%d.bak", path, fromVersion)
}

// migrateFromV1 copies the database aside and then, in one transaction, adds the leaf columns and
// fills them in from the stored envelopes. A copy already there is kept - it is the oldest state of
// the data, from a migration that did not finish - and the transaction means an interrupted
// migration leaves schema 1 as it was, to be migrated again on the next start.
func (s *DB) migrateFromV1(ctx context.Context, path string) error {
	backup := BackupPath(path, 1)
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := s.BackupTo(ctx, backup); err != nil {
			return fmt.Errorf("store: back up schema 1 before migrating: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("store: stat %s: %w", backup, err)
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, migrateV1Columns); err != nil {
			return fmt.Errorf("store: add leaf columns: %w", err)
		}
		if err := backfillLeaves(ctx, tx); err != nil {
			return err
		}
		return stampSchema(ctx, tx)
	})
}

// leafBackfillPage bounds how many envelopes the migration holds at once.
const leafBackfillPage = 512

type versionKey struct {
	vault, record []byte
	version       uint64
}

// backfillLeaves walks record_versions in key order a page at a time, so the migration's memory is
// one page however large the database is, and no row is updated while the query reading it is open.
func backfillLeaves(ctx context.Context, tx *sql.Tx) error {
	after := versionKey{vault: []byte{}, record: []byte{}}
	for {
		keys, envelopes, err := leafPage(ctx, tx, after)
		if err != nil || len(keys) == 0 {
			return err
		}
		for i, raw := range envelopes {
			e, err := decodeEnvelope(raw)
			if err != nil {
				return fmt.Errorf("store: migrate a stored envelope: %w", err)
			}
			digest, err := e.Digest()
			if err != nil {
				return err
			}
			k := keys[i]
			if _, err := tx.ExecContext(ctx, "UPDATE record_versions SET digest = ?, deleted = ? WHERE vault = ? AND record = ? AND version = ?",
				digest[:], e.Deleted, k.vault, k.record, k.version); err != nil {
				return fmt.Errorf("store: migrate a stored envelope: %w", err)
			}
		}
		after = keys[len(keys)-1]
	}
}

func leafPage(ctx context.Context, tx *sql.Tx, after versionKey) ([]versionKey, [][]byte, error) {
	rows, err := tx.QueryContext(ctx, `SELECT vault, record, version, envelope FROM record_versions
		WHERE (vault, record, version) > (?, ?, ?) ORDER BY vault, record, version LIMIT ?`,
		after.vault, after.record, after.version, leafBackfillPage)
	if err != nil {
		return nil, nil, fmt.Errorf("store: read envelopes to migrate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []versionKey
	var envelopes [][]byte
	for rows.Next() {
		var k versionKey
		var raw []byte
		if err := rows.Scan(&k.vault, &k.record, &k.version, &raw); err != nil {
			return nil, nil, err
		}
		keys, envelopes = append(keys, k), append(envelopes, raw)
	}
	return keys, envelopes, rows.Err()
}
