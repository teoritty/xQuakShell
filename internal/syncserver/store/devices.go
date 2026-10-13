package store

import (
	"encoding/json"
	"fmt"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

// Device log entries are stored in their wire form: the server keeps exactly the bytes it will
// serve, and decoding them on the way out goes through the same strict checks as a request.
func (t *txn) DeviceEntries(account vaultsync.ID) ([]vaultsync.DeviceEntry, error) {
	rows, err := t.tx.QueryContext(t.ctx, "SELECT entry FROM device_entries WHERE account = ? ORDER BY seq", account[:])
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []vaultsync.DeviceEntry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var d syncwire.DeviceEntry
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("store: stored device entry: %w", err)
		}
		e, err := d.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (t *txn) AppendDeviceEntries(account vaultsync.ID, entries []vaultsync.DeviceEntry) error {
	for i := range entries {
		raw, err := json.Marshal(syncwire.DeviceEntryFrom(&entries[i]))
		if err != nil {
			return err
		}
		if err := t.exec("INSERT INTO device_entries (account, seq, entry) VALUES (?, ?, ?)", account[:], entries[i].Seq, raw); err != nil {
			return err
		}
	}
	return nil
}

func (t *txn) UpsertDevice(d usecase.Device) error {
	return t.exec(`INSERT INTO devices (id, account, signing_key, revoked) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET revoked = excluded.revoked`, d.ID[:], d.Account[:], d.SigningKey, d.Revoked)
}

func (t *txn) DeviceByID(id vaultsync.ID) (usecase.Device, bool, error) {
	var d usecase.Device
	var account []byte
	err := t.tx.QueryRowContext(t.ctx, "SELECT account, signing_key, revoked FROM devices WHERE id = ?", id[:]).
		Scan(&account, &d.SigningKey, &d.Revoked)
	d.ID, d.Account = id, scanOptionalID(account)
	ok, err := found(err)
	return d, ok, err
}

func (t *txn) InsertToken(tok usecase.Token) error {
	return t.exec("INSERT INTO tokens (hash, account, device, kind, expires) VALUES (?, ?, ?, ?, ?)",
		tok.Hash, tok.Account[:], idOrNil(tok.Device), string(tok.Kind), tok.Expires)
}

func (t *txn) TokenByHash(hash []byte) (usecase.Token, bool, error) {
	var tok usecase.Token
	var account, device []byte
	var kind string
	err := t.tx.QueryRowContext(t.ctx, "SELECT account, device, kind, expires FROM tokens WHERE hash = ?", hash).
		Scan(&account, &device, &kind, &tok.Expires)
	tok.Hash, tok.Account, tok.Device, tok.Kind = hash, scanOptionalID(account), scanOptionalID(device), usecase.TokenKind(kind)
	ok, err := found(err)
	return tok, ok, err
}

func (t *txn) DeleteTokensForDevice(device vaultsync.ID) error {
	return t.exec("DELETE FROM tokens WHERE device = ?", device[:])
}

func (t *txn) DeleteExpiredTokens(now uint64) error {
	return t.exec("DELETE FROM tokens WHERE expires <= ?", now)
}
