package syncstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

// ErrForeignRecord is a record filed under another vault's head.
var ErrForeignRecord = errors.New("syncstate: record belongs to another vault")

// decodeAs parses a stored row through its wire DTO, so a row is held to exactly the checks a
// message from the server is: a corrupted or edited file fails here instead of reaching the engine.
func decodeAs[T any, D interface{ ToDomain() (T, error) }](raw []byte) (T, error) {
	var d D
	if err := json.Unmarshal(raw, &d); err != nil {
		var zero T
		return zero, fmt.Errorf("syncstate: decode row: %w", err)
	}
	return d.ToDomain()
}

func rows[T any](ctx context.Context, db *sql.DB, decode func([]byte) (T, error), query string, args ...any) ([]T, error) {
	rs, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("syncstate: query: %w", err)
	}
	defer func() { _ = rs.Close() }()
	var out []T
	for rs.Next() {
		var raw []byte
		if err := rs.Scan(&raw); err != nil {
			return nil, fmt.Errorf("syncstate: scan: %w", err)
		}
		v, err := decode(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rs.Err()
}

// LoadVault returns the verified state of one vault, empty when the device has none.
func (s *DB) LoadVault(ctx context.Context, vault vaultsync.ID) (synctrust.StoredVault, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT head FROM heads WHERE vault = ?", vault[:]).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return synctrust.StoredVault{}, nil
	}
	if err != nil {
		return synctrust.StoredVault{}, fmt.Errorf("syncstate: load head: %w", err)
	}
	head, err := decodeAs[vaultsync.Head, syncwire.Head](raw)
	if err != nil {
		return synctrust.StoredVault{}, err
	}
	records, err := rows(ctx, s.db, decodeAs[vaultsync.Envelope, syncwire.Envelope],
		"SELECT envelope FROM records WHERE vault = ? ORDER BY record", vault[:])
	if err != nil {
		return synctrust.StoredVault{}, err
	}
	landed, err := s.landedAt(ctx, vault)
	if err != nil {
		return synctrust.StoredVault{}, err
	}
	return synctrust.StoredVault{Head: &head, Records: records, LandedAt: landed}, nil
}

func (s *DB) landedAt(ctx context.Context, vault vaultsync.ID) (map[vaultsync.ID]uint64, error) {
	rs, err := s.db.QueryContext(ctx, "SELECT record, at FROM records WHERE vault = ?", vault[:])
	if err != nil {
		return nil, fmt.Errorf("syncstate: load landing times: %w", err)
	}
	defer func() { _ = rs.Close() }()
	out := map[vaultsync.ID]uint64{}
	for rs.Next() {
		var raw []byte
		var at uint64
		if err := rs.Scan(&raw, &at); err != nil {
			return nil, fmt.Errorf("syncstate: load landing times: %w", err)
		}
		var id vaultsync.ID
		copy(id[:], raw)
		out[id] = at
	}
	return out, rs.Err()
}

// AdvanceVault stores the records a verified commit changed, drops the ones it purged and moves the
// head, in one transaction. Each stored version is dated by the head that carried it, the time the
// trash window of a tombstone counts from.
func (s *DB) AdvanceVault(ctx context.Context, head vaultsync.Head, changed []vaultsync.Envelope, purged []vaultsync.ID) error {
	headRaw, err := json.Marshal(syncwire.HeadFrom(&head))
	if err != nil {
		return fmt.Errorf("syncstate: encode head: %w", err)
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var cur uint64
		err := tx.QueryRowContext(ctx, "SELECT seq FROM heads WHERE vault = ?", head.Vault[:]).Scan(&cur)
		switch {
		case err == nil && head.Seq <= cur:
			return fmt.Errorf("%w: %d after %d", synctrust.ErrStaleHead, head.Seq, cur)
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("syncstate: read head: %w", err)
		}
		for i := range changed {
			if err := putRecord(ctx, tx, &head, &changed[i]); err != nil {
				return err
			}
		}
		for _, id := range purged {
			if _, err := tx.ExecContext(ctx, "DELETE FROM records WHERE vault = ? AND record = ?", head.Vault[:], id[:]); err != nil {
				return fmt.Errorf("syncstate: purge record: %w", err)
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO heads (vault, seq, head) VALUES (?, ?, ?)",
			head.Vault[:], head.Seq, headRaw)
		return err
	})
}

func putRecord(ctx context.Context, tx *sql.Tx, head *vaultsync.Head, e *vaultsync.Envelope) error {
	if e.Vault != head.Vault {
		return ErrForeignRecord
	}
	raw, err := json.Marshal(syncwire.EnvelopeFrom(e))
	if err != nil {
		return fmt.Errorf("syncstate: encode record: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO records (vault, record, envelope, at) VALUES (?, ?, ?, ?)",
		head.Vault[:], e.Record[:], raw, head.ClientTimeUnixMilli)
	return err
}

// ForgetVault removes everything stored for a vault.
func (s *DB) ForgetVault(ctx context.Context, vault vaultsync.ID) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, table := range []string{"heads", "records", "member_ops", "outbox", "quarantine"} {
			// The table names come from the literal list above, never from input.
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE vault = ?", vault[:]); err != nil { // #nosec G202
				return fmt.Errorf("syncstate: forget %s: %w", table, err)
			}
		}
		return nil
	})
}

// stateTables is every table that holds sync state; ForgetAll names tables from this literal list
// only, never from input. A table added to the schema and not here would survive turning sync off,
// which TestForgetAllLeavesNoRowInAnyTable catches by reading the schema itself.
var stateTables = []string{"heads", "records", "member_ops", "outbox", "quarantine", "device_entries"}

// ForgetAll removes everything the store holds, in one transaction.
func (s *DB) ForgetAll(ctx context.Context) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, table := range stateTables {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil { // #nosec G202 -- table from stateTables
				return fmt.Errorf("syncstate: forget %s: %w", table, err)
			}
		}
		return nil
	})
}

// DeviceLog returns an account's device log in order.
func (s *DB) DeviceLog(ctx context.Context, account vaultsync.ID) ([]vaultsync.DeviceEntry, error) {
	return rows(ctx, s.db, decodeAs[vaultsync.DeviceEntry, syncwire.DeviceEntry],
		"SELECT entry FROM device_entries WHERE account = ? ORDER BY seq", account[:])
}

// AppendDeviceLog stores entries of one account that continue its stored log without a gap.
func (s *DB) AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return s.appendLog(ctx, deviceLog, entries[0].Account, len(entries), func(i int) (vaultsync.ID, uint64, any) {
		return entries[i].Account, entries[i].Seq, syncwire.DeviceEntryFrom(&entries[i])
	})
}

// Membership returns a vault's membership log in order.
func (s *DB) Membership(ctx context.Context, vault vaultsync.ID) ([]vaultsync.MemberOp, error) {
	return rows(ctx, s.db, decodeAs[vaultsync.MemberOp, syncwire.MemberOp],
		"SELECT op FROM member_ops WHERE vault = ? ORDER BY seq", vault[:])
}

// AppendMembership stores ops of one vault that continue its stored log without a gap.
func (s *DB) AppendMembership(ctx context.Context, ops []vaultsync.MemberOp) error {
	if len(ops) == 0 {
		return nil
	}
	return s.appendLog(ctx, memberLog, ops[0].Vault, len(ops), func(i int) (vaultsync.ID, uint64, any) {
		return ops[i].Vault, ops[i].Seq, syncwire.MemberOpFrom(&ops[i])
	})
}

// logTable names a log's table and its owner column. Only the two values below exist, so the names
// that reach SQL text are literals, never input.
type logTable struct{ table, owner string }

var (
	deviceLog = logTable{"device_entries", "account"}
	memberLog = logTable{"member_ops", "vault"}
)

// appendLog is the shared body of the two log appends; item returns each entry's owner, sequence
// number and wire form.
func (s *DB) appendLog(ctx context.Context, lt logTable, id vaultsync.ID, n int, item func(int) (vaultsync.ID, uint64, any)) error {
	table, owner := lt.table, lt.owner
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var last uint64
		// #nosec G202 -- table and owner come from deviceLog or memberLog.
		q := "SELECT coalesce(max(seq), 0) FROM " + table + " WHERE " + owner + " = ?"
		if err := tx.QueryRowContext(ctx, q, id[:]).Scan(&last); err != nil {
			return fmt.Errorf("syncstate: read %s: %w", table, err)
		}
		for i := 0; i < n; i++ {
			who, seq, wire := item(i)
			if who != id || seq != last+1 {
				return fmt.Errorf("%w: %s seq %d after %d", synctrust.ErrLogGap, table, seq, last)
			}
			raw, err := json.Marshal(wire)
			if err != nil {
				return fmt.Errorf("syncstate: encode %s: %w", table, err)
			}
			// #nosec G202 -- table comes from deviceLog or memberLog.
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" VALUES (?, ?, ?)", id[:], seq, raw); err != nil {
				return fmt.Errorf("syncstate: insert %s: %w", table, err)
			}
			last = seq
		}
		return nil
	})
}
