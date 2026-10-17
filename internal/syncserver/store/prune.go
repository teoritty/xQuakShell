package store

import (
	"encoding/json"
	"strings"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

// The pruning queries never touch a record's latest version: that is the state every snapshot and
// every Merkle root is built from. Only a purge, which removes the record from the tree, deletes it.

func (t *txn) AllVaults() ([]vaultsync.ID, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT id FROM vaults WHERE deleted = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []vaultsync.ID
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, scanOptionalID(raw))
	}
	return out, rows.Err()
}

// deleteVersions deletes the record_versions rows a predicate selects and reports the newest seq
// among them, so the caller can mark the history before it unverifiable.
func (t *txn) deleteVersions(where string, args ...any) (uint64, error) {
	var seq uint64
	if err := t.tx.QueryRowContext(t.ctx, "SELECT coalesce(max(rv.seq), 0) FROM record_versions rv WHERE "+where, args...).Scan(&seq); err != nil {
		return 0, err
	}
	if seq == 0 {
		return 0, nil
	}
	return seq, t.exec("DELETE FROM record_versions AS rv WHERE "+where, args...)
}

func (t *txn) DeleteRecords(vault vaultsync.ID, records []vaultsync.ID) (uint64, error) {
	var newest uint64
	for _, r := range records {
		seq, err := t.deleteVersions("rv.vault = ? AND rv.record = ?", vault[:], r[:])
		if err != nil {
			return 0, err
		}
		newest = max(newest, seq)
	}
	return newest, nil
}

// supersededAt selects versions that have a successor; the successor's commit time is when the
// version became history, which is what the trash window counts from - not when it was written, or
// a record's last live version would vanish the moment it was deleted.
const supersededAt = `rv.vault = ? AND EXISTS (SELECT 1 FROM record_versions nx JOIN commits c ON c.vault = nx.vault AND c.seq = nx.seq
	WHERE nx.vault = rv.vault AND nx.record = rv.record AND nx.version = rv.version + 1 AND c.received < ?)`

func (t *txn) DeleteSuperseded(vault vaultsync.ID, cutoff uint64) (uint64, error) {
	return t.deleteVersions(supersededAt, vault[:], cutoff)
}

func (t *txn) DeleteEpochVersions(vault vaultsync.ID, epochs []uint32) (uint64, error) {
	if len(epochs) == 0 {
		return 0, nil
	}
	args := []any{vault[:]}
	for _, e := range epochs {
		args = append(args, e)
	}
	where := `rv.vault = ? AND rv.epoch IN (?` + strings.Repeat(", ?", len(epochs)-1) + `)
		AND rv.version < (SELECT max(version) FROM record_versions WHERE vault = rv.vault AND record = rv.record)`
	return t.deleteVersions(where, args...)
}

// ReplaceMemberOps rewrites stored ops in place, for stripping retired wraps. The caller passes ops
// that still verify; the seq of each names the row it replaces.
func (t *txn) ReplaceMemberOps(vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	for i := range ops {
		raw, err := json.Marshal(syncwire.MemberOpFrom(&ops[i]))
		if err != nil {
			return err
		}
		if err := t.exec("UPDATE member_ops SET op = ? WHERE vault = ? AND seq = ?", raw, vault[:], ops[i].Seq); err != nil {
			return err
		}
	}
	return nil
}

func (t *txn) RaisePrunedSeq(vault vaultsync.ID, seq uint64) error {
	return t.exec("UPDATE vaults SET pruned_seq = max(pruned_seq, ?) WHERE id = ?", seq, vault[:])
}

func (t *txn) RecomputeBytes(vault vaultsync.ID) error {
	return t.exec("UPDATE vaults SET bytes = (SELECT coalesce(sum(size), 0) FROM record_versions WHERE vault = ?) WHERE id = ?", vault[:], vault[:])
}
