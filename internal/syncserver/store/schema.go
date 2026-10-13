package store

import (
	"context"
	"errors"
	"fmt"
)

// schemaVersion is the server database shape this build reads and writes (ADR-023 server store
// schema axis). Any bump needs a migration that backs the file up first, as ADR-017 §4 requires.
const schemaVersion = 1

// setSchemaVersionSQL stamps the version; PRAGMA takes no bound parameter, so it is a literal.
const setSchemaVersionSQL = "PRAGMA user_version = 1"

var (
	// ErrStoreTooNew is a database written by a newer server. It is never opened for writing:
	// writing back only the columns this build knows would drop the rest.
	ErrStoreTooNew = errors.New("store: database written by a newer xqsyncd")
	// ErrStoreUnknown is a non-empty database without a schema version - not one of ours.
	ErrStoreUnknown = errors.New("store: database was not created by xqsyncd")
)

const schemaV1 = `
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
  envelope BLOB NOT NULL, PRIMARY KEY (vault, record, version));
CREATE INDEX record_versions_seq ON record_versions (vault, seq);
`

// ensureSchema creates the schema in a fresh database. An existing one at this version needs
// nothing; checkExisting has already refused every other case.
func (s *DB) ensureSchema(ctx context.Context) error {
	version, tables, err := inspect(ctx, s.db)
	if err != nil {
		return fmt.Errorf("store: inspect: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 || tables != 0 {
		return ErrStoreUnknown
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: create schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, setSchemaVersionSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: stamp schema: %w", err)
	}
	return tx.Commit()
}
