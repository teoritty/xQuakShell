package store

import (
	"encoding/json"
	"fmt"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

const vaultColumns = "id, owner, seq, membership_seq, bytes, personal, deleted, pruned_seq"

func (t *txn) InsertVault(v usecase.Vault) error {
	return t.exec("INSERT INTO vaults ("+vaultColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		v.ID[:], v.Owner[:], v.Seq, v.MembershipSeq, v.Bytes, v.Personal, v.Deleted, v.PrunedSeq)
}

func scanVault(row interface{ Scan(...any) error }) (usecase.Vault, error) {
	var v usecase.Vault
	var id, owner []byte
	err := row.Scan(&id, &owner, &v.Seq, &v.MembershipSeq, &v.Bytes, &v.Personal, &v.Deleted, &v.PrunedSeq)
	v.ID, v.Owner = scanOptionalID(id), scanOptionalID(owner)
	return v, err
}

func (t *txn) VaultByID(id vaultsync.ID) (usecase.Vault, bool, error) {
	v, err := scanVault(t.tx.QueryRowContext(t.ctx, "SELECT "+vaultColumns+" FROM vaults WHERE id = ?", id[:]))
	ok, err := found(err)
	return v, ok, err
}

func (t *txn) UpdateVaultHead(id vaultsync.ID, seq, bytes uint64) error {
	return t.exec("UPDATE vaults SET seq = ?, bytes = ? WHERE id = ?", seq, bytes, id[:])
}

func (t *txn) SetVaultMembershipSeq(id vaultsync.ID, seq uint64) error {
	return t.exec("UPDATE vaults SET membership_seq = ? WHERE id = ?", seq, id[:])
}

func (t *txn) MemberOps(vault vaultsync.ID) ([]vaultsync.MemberOp, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT op FROM member_ops WHERE vault = ? ORDER BY seq", vault[:])
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []vaultsync.MemberOp
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var d syncwire.MemberOp
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("store: stored membership op: %w", err)
		}
		op, err := d.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

func (t *txn) AppendMemberOps(vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	for i := range ops {
		raw, err := json.Marshal(syncwire.MemberOpFrom(&ops[i]))
		if err != nil {
			return err
		}
		if err := t.exec("INSERT INTO member_ops (vault, seq, op) VALUES (?, ?, ?)", vault[:], ops[i].Seq, raw); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceMembers rewrites the derived member table from a verified replay of the log. It is a
// cache for authorization, never a source of truth over the log itself.
func (t *txn) ReplaceMembers(vault vaultsync.ID, roles map[vaultsync.ID]vaultsync.Role) error {
	if err := t.exec("DELETE FROM members WHERE vault = ?", vault[:]); err != nil {
		return err
	}
	for account, role := range roles {
		if err := t.exec("INSERT INTO members (vault, account, role) VALUES (?, ?, ?)", vault[:], account[:], int(role)); err != nil {
			return err
		}
	}
	return nil
}

func (t *txn) MemberRole(vault, account vaultsync.ID) (vaultsync.Role, error) {
	var role int
	err := t.tx.QueryRowContext(t.ctx, "SELECT role FROM members WHERE vault = ? AND account = ?", vault[:], account[:]).Scan(&role)
	if ok, err := found(err); !ok {
		return vaultsync.RoleNone, err
	}
	return vaultsync.Role(role), nil // #nosec G115 -- roles are written by this package from vaultsync.Role values
}

func (t *txn) VaultsOf(account vaultsync.ID) ([]usecase.Vault, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT v."+vaultColumnsQualified+" FROM vaults v JOIN members m ON m.vault = v.id WHERE m.account = ? AND v.deleted = 0 ORDER BY v.id", account[:])
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []usecase.Vault
	for rows.Next() {
		v, err := scanVault(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const vaultColumnsQualified = "id, v.owner, v.seq, v.membership_seq, v.bytes, v.personal, v.deleted, v.pruned_seq"

func (t *txn) SharesVault(a, b vaultsync.ID) (bool, error) {
	var n int
	err := t.tx.QueryRowContext(t.ctx, `SELECT count(*) FROM members ma JOIN members mb ON ma.vault = mb.vault
		WHERE ma.account = ? AND mb.account = ?`, a[:], b[:]).Scan(&n)
	return n > 0, err
}
