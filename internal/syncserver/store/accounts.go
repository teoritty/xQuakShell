package store

import (
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

const accountColumns = "id, username, opaque_record, bundle, identity_key, encryption_key, created, frozen"

func (t *txn) InsertAccount(a usecase.Account) error {
	return t.exec("INSERT INTO accounts ("+accountColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		a.ID[:], a.Username, a.OPAQUERecord, a.Bundle, a.IdentityKey, a.EncryptionKey, a.Created, a.Frozen)
}

func scanAccount(row interface{ Scan(...any) error }) (usecase.Account, error) {
	var a usecase.Account
	var id []byte
	err := row.Scan(&id, &a.Username, &a.OPAQUERecord, &a.Bundle, &a.IdentityKey, &a.EncryptionKey, &a.Created, &a.Frozen)
	copy(a.ID[:], id)
	return a, err
}

func (t *txn) AccountByUsername(username string) (usecase.Account, bool, error) {
	a, err := scanAccount(t.tx.QueryRowContext(t.ctx, "SELECT "+accountColumns+" FROM accounts WHERE username = ?", username))
	ok, err := found(err)
	return a, ok, err
}

func (t *txn) AccountByID(id vaultsync.ID) (usecase.Account, bool, error) {
	a, err := scanAccount(t.tx.QueryRowContext(t.ctx, "SELECT "+accountColumns+" FROM accounts WHERE id = ?", id[:]))
	ok, err := found(err)
	return a, ok, err
}

func (t *txn) SetFrozen(username string, frozen bool) (bool, error) {
	return t.changed("UPDATE accounts SET frozen = ? WHERE username = ?", frozen, username)
}

func (t *txn) DeleteAccount(username string) (bool, error) {
	return t.changed("DELETE FROM accounts WHERE username = ?", username)
}

func (t *txn) ListAccounts() ([]usecase.Account, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT "+accountColumns+" FROM accounts ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []usecase.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (t *txn) InsertInvite(codeHash []byte, expires uint64) error {
	return t.exec("INSERT INTO invites (code_hash, expires) VALUES (?, ?)", codeHash, expires)
}

// InviteUsable reports an unexpired, unused invite without consuming it, so registration can refuse
// early without the code being spent on a request that then fails.
func (t *txn) InviteUsable(codeHash []byte, now uint64) (bool, error) {
	var n int
	err := t.tx.QueryRowContext(t.ctx, "SELECT count(*) FROM invites WHERE code_hash = ? AND used_by IS NULL AND expires > ?", codeHash, now).Scan(&n)
	return n > 0, err
}

// ConsumeInvite marks an unexpired, unused invite as used by account, in one statement so two
// registrations racing for one code cannot both succeed.
func (t *txn) ConsumeInvite(codeHash []byte, now uint64, account vaultsync.ID) (bool, error) {
	return t.changed("UPDATE invites SET used_by = ? WHERE code_hash = ? AND used_by IS NULL AND expires > ?",
		account[:], codeHash, now)
}

func idOrNil(id vaultsync.ID) any {
	if id.IsZero() {
		return nil
	}
	return id[:]
}

func scanOptionalID(b []byte) vaultsync.ID {
	var id vaultsync.ID
	copy(id[:], b)
	return id
}

// UpdateAccountCredentials replaces the OPAQUE record and the bundle after a password change.
func (t *txn) UpdateAccountCredentials(account vaultsync.ID, record, bundle []byte) error {
	ok, err := t.changed("UPDATE accounts SET opaque_record = ?, bundle = ? WHERE id = ?", record, bundle, account[:])
	if err == nil && !ok {
		return usecase.ErrNotFound
	}
	return err
}
