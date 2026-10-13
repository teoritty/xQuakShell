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

// PutOutbox records the commit about to be sent. There is at most one per vault: a second would be
// built on a head the first has not reached yet, and the server could only refuse it.
func (s *DB) PutOutbox(ctx context.Context, c vaultsync.Commit) error {
	wire := syncwire.Commit{ExpectSeq: c.Head.Seq - 1, Head: syncwire.HeadFrom(&c.Head)}
	for i := range c.Records {
		wire.Records = append(wire.Records, syncwire.EnvelopeFrom(&c.Records[i]))
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("syncstate: encode outbox: %w", err)
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE vault = ?", c.Head.Vault[:]).Scan(&n); err != nil {
			return fmt.Errorf("syncstate: read outbox: %w", err)
		}
		if n > 0 {
			return synctrust.ErrOutboxBusy
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO outbox (vault, commit_id, payload) VALUES (?, ?, ?)",
			c.Head.Vault[:], c.Head.Commit[:], raw)
		return err
	})
}

// Outbox returns the pending commit of a vault, or nil.
func (s *DB) Outbox(ctx context.Context, vault vaultsync.ID) (*vaultsync.Commit, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT payload FROM outbox WHERE vault = ?", vault[:]).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("syncstate: read outbox: %w", err)
	}
	var wire syncwire.Commit
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("syncstate: decode outbox: %w", err)
	}
	head, err := wire.Head.ToDomain()
	if err != nil {
		return nil, err
	}
	c := &vaultsync.Commit{Head: head}
	for _, w := range wire.Records {
		e, err := w.ToDomain()
		if err != nil {
			return nil, err
		}
		c.Records = append(c.Records, e)
	}
	return c, nil
}

// ClearOutbox drops the pending commit once the server acknowledged it. It matches the commit id, so
// an acknowledgement that arrives late for an older commit cannot drop a newer pending one.
func (s *DB) ClearOutbox(ctx context.Context, vault, commit vaultsync.ID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM outbox WHERE vault = ? AND commit_id = ?", vault[:], commit[:])
	return err
}

// Quarantine stores a record that verified but was not applied, replacing an older quarantined
// version of the same record.
func (s *DB) Quarantine(ctx context.Context, item synctrust.QuarantineItem) error {
	e := item.Envelope
	raw, err := json.Marshal(syncwire.EnvelopeFrom(&e))
	if err != nil {
		return fmt.Errorf("syncstate: encode quarantine: %w", err)
	}
	_, err = s.db.ExecContext(ctx, "INSERT OR REPLACE INTO quarantine (vault, record, envelope, reason, at) VALUES (?, ?, ?, ?, ?)",
		e.Vault[:], e.Record[:], raw, item.Reason, item.AtUnixMilli)
	return err
}

// Quarantined lists a vault's quarantined records.
func (s *DB) Quarantined(ctx context.Context, vault vaultsync.ID) ([]synctrust.QuarantineItem, error) {
	rs, err := s.db.QueryContext(ctx, "SELECT envelope, reason, at FROM quarantine WHERE vault = ? ORDER BY record", vault[:])
	if err != nil {
		return nil, fmt.Errorf("syncstate: query quarantine: %w", err)
	}
	defer func() { _ = rs.Close() }()
	var out []synctrust.QuarantineItem
	for rs.Next() {
		var raw []byte
		var item synctrust.QuarantineItem
		if err := rs.Scan(&raw, &item.Reason, &item.AtUnixMilli); err != nil {
			return nil, fmt.Errorf("syncstate: scan quarantine: %w", err)
		}
		if item.Envelope, err = decodeAs[vaultsync.Envelope, syncwire.Envelope](raw); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rs.Err()
}

// Release drops a record from quarantine.
func (s *DB) Release(ctx context.Context, vault, record vaultsync.ID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM quarantine WHERE vault = ? AND record = ?", vault[:], record[:])
	return err
}
