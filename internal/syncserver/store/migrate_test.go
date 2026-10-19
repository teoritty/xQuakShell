package store_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/usecase"
)

// schemaV1 is the database as schema 1 created it, before record_versions had leaf columns.
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
PRAGMA user_version = 1;
`

func migrationEnvelopes() []vaultsync.Envelope {
	live := vaultsync.Envelope{Vault: vaultsync.ID{9}, Record: vaultsync.ID{1}, Version: 1, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{1}, 72), Body: bytes.Repeat([]byte{1}, 296), Author: vaultsync.ID{5},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{7}, 64)}
	tomb := vaultsync.Envelope{Vault: vaultsync.ID{9}, Record: vaultsync.ID{2}, Version: 2, Suite: vaultsync.Suite1,
		Deleted: true, Author: vaultsync.ID{5}, SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	return []vaultsync.Envelope{live, tomb}
}

// writeSchemaV1 builds a database as a schema 1 server left it, holding the given envelopes.
func writeSchemaV1(t *testing.T, path string, envs []vaultsync.Envelope) {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, schemaV1); err != nil {
		t.Fatal(err)
	}
	vault, owner := vaultsync.ID{9}, vaultsync.ID{1}
	if _, err := raw.ExecContext(ctx, "INSERT INTO vaults (id, owner) VALUES (?, ?)", vault[:], owner[:]); err != nil {
		t.Fatal(err)
	}
	for i := range envs {
		e := &envs[i]
		blob, _ := json.Marshal(syncwire.EnvelopeFrom(e))
		if _, err := raw.ExecContext(ctx, "INSERT INTO record_versions (vault, record, version, seq, epoch, size, envelope) VALUES (?, ?, ?, 1, 0, ?, ?)",
			e.Vault[:], e.Record[:], e.Version, len(e.WrappedDEK)+len(e.Body), blob); err != nil {
			t.Fatal(err)
		}
	}
}

// A schema 1 database is copied aside, then gains a leaf for every stored version - the digest of
// its envelope and its tombstone flag - computed from the envelope itself.
func TestOpenMigratesSchema1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xqsyncd.db")
	envs := migrationEnvelopes()
	writeSchemaV1(t, path, envs)
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open schema 1: %v", err)
	}
	defer func() { _ = db.Close() }()
	if v, _ := db.SchemaVersion(ctx); v != 2 {
		t.Fatalf("schema after migration = %d, want 2", v)
	}
	var leaves []vaultsync.Leaf
	if err := db.WithTx(ctx, func(tx usecase.Tx) error {
		var err error
		leaves, err = tx.CurrentLeaves(vaultsync.ID{9})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(leaves) != len(envs) {
		t.Fatalf("%d leaves after migration, want %d", len(leaves), len(envs))
	}
	for _, l := range leaves {
		for i := range envs {
			if envs[i].Record != l.Record {
				continue
			}
			want, _ := vaultsync.LeafOf(&envs[i])
			if l != want {
				t.Fatalf("migrated leaf %+v, want %+v from the envelope", l, want)
			}
		}
	}
	copyDB, err := sql.Open("sqlite", "file:"+store.BackupPath(path, 1)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copyDB.Close() }()
	var version, rows int
	if err := copyDB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("the copy beside the migrated database is at schema %d (%v); want the schema 1 data", version, err)
	}
	if err := copyDB.QueryRowContext(ctx, "SELECT count(*) FROM record_versions").Scan(&rows); err != nil || rows != len(envs) {
		t.Fatalf("the schema 1 copy holds %d versions (%v), want %d", rows, err, len(envs))
	}
}

// A copy left by an earlier migration attempt is the oldest state of the data and is never
// overwritten.
func TestMigrationKeepsAnExistingBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xqsyncd.db")
	writeSchemaV1(t, path, migrationEnvelopes())
	marker := []byte("an earlier copy")
	if err := os.WriteFile(store.BackupPath(path, 1), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open schema 1 with a copy already there: %v", err)
	}
	_ = db.Close()
	if got, _ := os.ReadFile(store.BackupPath(path, 1)); !bytes.Equal(got, marker) {
		t.Fatal("an existing schema 1 copy was overwritten")
	}
}
