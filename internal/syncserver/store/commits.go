package store

import (
	"encoding/json"
	"fmt"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

func (t *txn) CommitSeqByID(vault, commitID vaultsync.ID) (uint64, bool, error) {
	var seq uint64
	err := t.tx.QueryRowContext(t.ctx, "SELECT seq FROM commits WHERE vault = ? AND commit_id = ?", vault[:], commitID[:]).Scan(&seq)
	ok, err := found(err)
	return seq, ok, err
}

// InsertCommit stores the head and every record version it carries, all tagged with seq so a pull
// can return whole commits.
func (t *txn) InsertCommit(vault vaultsync.ID, seq uint64, c vaultsync.Commit) error {
	head, err := json.Marshal(syncwire.HeadFrom(&c.Head))
	if err != nil {
		return err
	}
	if err := t.exec("INSERT INTO commits (vault, seq, commit_id, head) VALUES (?, ?, ?, ?)", vault[:], seq, c.Head.Commit[:], head); err != nil {
		return err
	}
	for i := range c.Records {
		raw, err := json.Marshal(syncwire.EnvelopeFrom(&c.Records[i]))
		if err != nil {
			return err
		}
		e := &c.Records[i]
		if err := t.exec("INSERT INTO record_versions (vault, record, version, seq, envelope) VALUES (?, ?, ?, ?, ?)",
			vault[:], e.Record[:], e.Version, seq, raw); err != nil {
			return err
		}
	}
	return nil
}

func decodeHead(raw []byte) (vaultsync.Head, error) {
	var d syncwire.Head
	if err := json.Unmarshal(raw, &d); err != nil {
		return vaultsync.Head{}, fmt.Errorf("store: stored head: %w", err)
	}
	return d.ToDomain()
}

func decodeEnvelope(raw []byte) (vaultsync.Envelope, error) {
	var d syncwire.Envelope
	if err := json.Unmarshal(raw, &d); err != nil {
		return vaultsync.Envelope{}, fmt.Errorf("store: stored envelope: %w", err)
	}
	return d.ToDomain()
}

func (t *txn) Commits(vault vaultsync.ID, since uint64, limit int) ([]usecase.StoredCommit, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT seq, head FROM commits WHERE vault = ? AND seq > ? ORDER BY seq LIMIT ?", vault[:], since, limit)
	if err != nil {
		return nil, err
	}
	var out []usecase.StoredCommit
	for rows.Next() {
		var sc usecase.StoredCommit
		var raw []byte
		if err := rows.Scan(&sc.Seq, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if sc.Commit.Head, err = decodeHead(raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, sc)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Commit.Records, err = t.recordsAt(vault, out[i].Seq); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (t *txn) recordsAt(vault vaultsync.ID, seq uint64) ([]vaultsync.Envelope, error) {
	return t.envelopes("SELECT envelope FROM record_versions WHERE vault = ? AND seq = ? ORDER BY record", vault[:], seq)
}

// CurrentRecords returns the latest version of every record - the state a new device starts from.
func (t *txn) CurrentRecords(vault vaultsync.ID) ([]vaultsync.Envelope, error) {
	return t.envelopes(`SELECT rv.envelope FROM record_versions rv
		WHERE rv.vault = ? AND rv.version = (SELECT max(version) FROM record_versions WHERE vault = rv.vault AND record = rv.record)
		ORDER BY rv.record`, vault[:])
}

func (t *txn) envelopes(query string, args ...any) ([]vaultsync.Envelope, error) {
	rows, err := t.tx.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []vaultsync.Envelope
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		e, err := decodeEnvelope(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (t *txn) LatestHead(vault vaultsync.ID) (vaultsync.Head, bool, error) {
	var raw []byte
	err := t.tx.QueryRowContext(t.ctx, "SELECT head FROM commits WHERE vault = ? ORDER BY seq DESC LIMIT 1", vault[:]).Scan(&raw)
	if ok, err := found(err); !ok {
		return vaultsync.Head{}, false, err
	}
	h, err := decodeHead(raw)
	return h, err == nil, err
}
