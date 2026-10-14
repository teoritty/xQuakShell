package store

import (
	"strings"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

func (t *txn) ListInvites() ([]usecase.InviteInfo, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT lower(hex(code_hash)), expires, used_by IS NOT NULL FROM invites ORDER BY expires")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []usecase.InviteInfo
	for rows.Next() {
		var inv usecase.InviteInfo
		if err := rows.Scan(&inv.ID, &inv.Expires, &inv.Used); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteUnusedInvite matches on the hash hex with substr rather than LIKE, so nothing in prefix can
// act as a wildcard even if a caller skipped the use case's check.
func (t *txn) DeleteUnusedInvite(prefix string) (int, error) {
	res, err := t.tx.ExecContext(t.ctx, "DELETE FROM invites WHERE used_by IS NULL AND substr(lower(hex(code_hash)), 1, ?) = ?",
		len(prefix), strings.ToLower(prefix))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (t *txn) Stats() (usecase.ServerStats, error) {
	var st usecase.ServerStats
	err := t.tx.QueryRowContext(t.ctx, `SELECT
		(SELECT count(*) FROM accounts), (SELECT count(*) FROM accounts WHERE frozen != 0),
		(SELECT count(*) FROM vaults WHERE deleted = 0), (SELECT count(*) FROM commits),
		(SELECT coalesce(sum(bytes), 0) FROM vaults WHERE deleted = 0)`).Scan(&st.Accounts, &st.Frozen, &st.Vaults, &st.Commits, &st.Bytes)
	return st, err
}

func (t *txn) MemberCount(vault vaultsync.ID) (int, error) {
	var n int
	err := t.tx.QueryRowContext(t.ctx, "SELECT count(*) FROM members WHERE vault = ?", vault[:]).Scan(&n)
	return n, err
}

// DeleteVault relies on ON DELETE CASCADE for commits, record versions, member ops, members and
// participants; secure_delete overwrites the ciphertext in the file.
func (t *txn) DeleteVault(vault vaultsync.ID) error {
	return t.exec("DELETE FROM vaults WHERE id = ?", vault[:])
}
