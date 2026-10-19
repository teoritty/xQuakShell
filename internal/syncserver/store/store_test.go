package store_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/usecase"
)

var ctx = context.Background()

func openTemp(t *testing.T) (*store.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xqsyncd.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func tx(t *testing.T, db *store.DB, fn func(usecase.Tx) error) {
	t.Helper()
	if err := db.WithTx(ctx, fn); err != nil {
		t.Fatal(err)
	}
}

func account(id byte, name string) usecase.Account {
	return usecase.Account{ID: vaultsync.ID{id}, Username: name, OPAQUERecord: []byte("record"), Bundle: []byte("bundle"),
		IdentityKey: bytes.Repeat([]byte{1}, 32), EncryptionKey: bytes.Repeat([]byte{2}, 1216), Created: 5}
}

func TestOpenCreatesSchemaVersion1(t *testing.T) {
	db, path := openTemp(t)
	if v, err := db.SchemaVersion(ctx); err != nil || v != 1 {
		t.Fatalf("schema version = %d, %v; want 1", v, err)
	}
	_ = db.Close()
	again, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening a database this build created: %v", err)
	}
	_ = again.Close()
}

func TestRefusesNewerSchemaWithoutTouchingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xqsyncd.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "CREATE TABLE future (x INTEGER); PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	before, _ := os.ReadFile(path)
	if _, err := store.Open(ctx, path); !errors.Is(err, store.ErrStoreTooNew) {
		t.Fatalf("err = %v, want ErrStoreTooNew; a newer server's data must not be opened", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("refusing a newer database still modified the file")
	}
}

func TestRefusesUnknownDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	raw, _ := sql.Open("sqlite", path)
	if _, err := raw.ExecContext(ctx, "CREATE TABLE notours (x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if _, err := store.Open(ctx, path); !errors.Is(err, store.ErrStoreUnknown) {
		t.Fatalf("err = %v, want ErrStoreUnknown for a database this program did not create", err)
	}
}

func TestSecureDeleteIsOn(t *testing.T) {
	db, _ := openTemp(t)
	on, err := db.SecureDelete(ctx)
	if err != nil || !on {
		t.Fatalf("secure_delete = %v, %v; deleted pages must be overwritten", on, err)
	}
}

// A deleted account's ciphertext must not survive in the database file: spec section 12.4.
func TestDeletedSecretsLeaveTheFile(t *testing.T) {
	db, path := openTemp(t)
	marker := bytes.Repeat([]byte("EXHUME-ME-"), 7)
	a := account(1, "alice")
	a.Bundle = marker
	tx(t, db, func(tx usecase.Tx) error { return tx.InsertAccount(a) })
	tx(t, db, func(tx usecase.Tx) error {
		_, err := tx.DeleteAccount("alice")
		return err
	})
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for _, p := range []string{path, path + "-wal"} {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if bytes.Contains(content, marker) {
			t.Fatalf("%s still contains a deleted bundle", filepath.Base(p))
		}
	}
}

func TestAccounts(t *testing.T) {
	db, _ := openTemp(t)
	tx(t, db, func(tx usecase.Tx) error { return tx.InsertAccount(account(1, "alice")) })
	err := db.WithTx(ctx, func(tx usecase.Tx) error { return tx.InsertAccount(account(2, "alice")) })
	if err == nil {
		t.Fatal("a second account with one username was accepted")
	}
	tx(t, db, func(tx usecase.Tx) error {
		got, ok, err := tx.AccountByUsername("alice")
		if err != nil || !ok || got.ID != (vaultsync.ID{1}) || string(got.Bundle) != "bundle" {
			t.Fatalf("by username = %+v, %v, %v", got, ok, err)
		}
		if _, ok, _ := tx.AccountByID(vaultsync.ID{9}); ok {
			t.Fatal("an unknown id was found")
		}
		if changed, err := tx.SetFrozen("alice", true); err != nil || !changed {
			t.Fatalf("freeze = %v, %v", changed, err)
		}
		got, _, _ = tx.AccountByID(vaultsync.ID{1})
		if !got.Frozen {
			t.Fatal("freeze did not persist")
		}
		list, err := tx.ListAccounts()
		if err != nil || len(list) != 1 {
			t.Fatalf("list = %d, %v", len(list), err)
		}
		return nil
	})
}

func TestTransactionRollsBack(t *testing.T) {
	db, _ := openTemp(t)
	boom := errors.New("boom")
	err := db.WithTx(ctx, func(tx usecase.Tx) error {
		if err := tx.InsertAccount(account(1, "alice")); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	tx(t, db, func(tx usecase.Tx) error {
		if _, ok, _ := tx.AccountByUsername("alice"); ok {
			t.Fatal("a failed transaction left its insert behind")
		}
		return nil
	})
}

func TestInvitesAreSingleUseAndExpire(t *testing.T) {
	db, _ := openTemp(t)
	tx(t, db, func(tx usecase.Tx) error { return tx.InsertInvite([]byte("h1"), 100) })
	tx(t, db, func(tx usecase.Tx) error {
		if ok, err := tx.ConsumeInvite([]byte("h1"), 101, vaultsync.ID{1}); err != nil || ok {
			t.Fatalf("expired invite consumed: %v, %v", ok, err)
		}
		return tx.InsertInvite([]byte("h2"), 1000)
	})
	tx(t, db, func(tx usecase.Tx) error {
		if ok, _ := tx.InviteUsable([]byte("h2"), 50); !ok {
			t.Fatal("a fresh invite is not usable")
		}
		if ok, err := tx.ConsumeInvite([]byte("h2"), 50, vaultsync.ID{1}); err != nil || !ok {
			t.Fatalf("valid invite refused: %v, %v", ok, err)
		}
		if ok, _ := tx.ConsumeInvite([]byte("h2"), 50, vaultsync.ID{2}); ok {
			t.Fatal("an invite was consumed twice")
		}
		if ok, _ := tx.InviteUsable([]byte("h2"), 50); ok {
			t.Fatal("a consumed invite is still reported usable")
		}
		return nil
	})
}

// A device row belongs to the account that first certified its id. Another account inserting the
// same id or revoking it changes nothing - the upsert this replaced let either flip the revoked flag.
func TestDeviceRowsStayWithTheirAccount(t *testing.T) {
	db, _ := openTemp(t)
	key := make([]byte, 32)
	tx(t, db, func(tx usecase.Tx) error {
		for i, name := range []string{"alice", "mallory"} {
			if err := tx.InsertAccount(account(byte(i+1), name)); err != nil {
				return err
			}
		}
		if ok, err := tx.InsertDevice(usecase.Device{ID: vaultsync.ID{7}, Account: vaultsync.ID{1}, SigningKey: key}); err != nil || !ok {
			t.Fatalf("first insert = %v, %v", ok, err)
		}
		if ok, err := tx.InsertDevice(usecase.Device{ID: vaultsync.ID{7}, Account: vaultsync.ID{2}, SigningKey: key}); err != nil || ok {
			t.Fatalf("a second account inserting a taken id = %v, %v; want false", ok, err)
		}
		if ok, err := tx.RevokeDevice(vaultsync.ID{2}, vaultsync.ID{7}); err != nil || ok {
			t.Fatalf("another account revoking the device = %v, %v; want false", ok, err)
		}
		d, _, _ := tx.DeviceByID(vaultsync.ID{7})
		if d.Account != (vaultsync.ID{1}) || d.Revoked {
			t.Fatalf("device after another account's attempts = %+v; want alice's, not revoked", d)
		}
		if ok, err := tx.RevokeDevice(vaultsync.ID{1}, vaultsync.ID{7}); err != nil || !ok {
			t.Fatalf("the owner revoking = %v, %v", ok, err)
		}
		if ok, _ := tx.InsertDevice(usecase.Device{ID: vaultsync.ID{7}, Account: vaultsync.ID{2}, SigningKey: key}); ok {
			t.Fatal("certifying a revoked id again was accepted")
		}
		if d, _, _ := tx.DeviceByID(vaultsync.ID{7}); !d.Revoked {
			t.Fatal("a revoked device was restored by inserting its id again")
		}
		return nil
	})
}

func TestDeviceLogsDevicesTokens(t *testing.T) {
	db, _ := openTemp(t)
	e1 := vaultsync.DeviceEntry{Account: vaultsync.ID{1}, Seq: 1, Op: vaultsync.OpAddDevice, SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64),
		Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{7}, SigningKey: make([]byte, 32)}}
	e2 := e1
	e2.Seq, e2.Cert = 2, &vaultsync.DeviceCert{Device: vaultsync.ID{8}, SigningKey: make([]byte, 32)}
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.InsertAccount(account(1, "alice")); err != nil {
			return err
		}
		if err := tx.AppendDeviceEntries(vaultsync.ID{1}, []vaultsync.DeviceEntry{e1, e2}); err != nil {
			return err
		}
		if ok, err := tx.InsertDevice(usecase.Device{ID: vaultsync.ID{7}, Account: vaultsync.ID{1}, SigningKey: make([]byte, 32)}); err != nil || !ok {
			t.Fatalf("insert device = %v, %v", ok, err)
		}
		return tx.InsertToken(usecase.Token{Hash: []byte("t"), Account: vaultsync.ID{1}, Device: vaultsync.ID{7}, Kind: usecase.TokenAccess, Expires: 10})
	})
	tx(t, db, func(tx usecase.Tx) error {
		got, err := tx.DeviceEntries(vaultsync.ID{1})
		if err != nil || len(got) != 2 || got[1].Cert.Device != (vaultsync.ID{8}) {
			t.Fatalf("entries = %+v, %v", got, err)
		}
		if tok, ok, _ := tx.TokenByHash([]byte("t")); !ok || tok.Kind != usecase.TokenAccess || tok.Device != (vaultsync.ID{7}) {
			t.Fatalf("token = %+v, %v", tok, ok)
		}
		if ok, err := tx.RevokeDevice(vaultsync.ID{1}, vaultsync.ID{7}); err != nil || !ok {
			t.Fatalf("revoke device = %v, %v", ok, err)
		}
		if d, _, _ := tx.DeviceByID(vaultsync.ID{7}); !d.Revoked {
			t.Fatal("device revocation did not persist")
		}
		if err := tx.DeleteTokensForDevice(vaultsync.ID{7}); err != nil {
			return err
		}
		if _, ok, _ := tx.TokenByHash([]byte("t")); ok {
			t.Fatal("a revoked device's token survived")
		}
		return nil
	})
}

func TestExpiredTokensAreDeleted(t *testing.T) {
	db, _ := openTemp(t)
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.InsertAccount(account(1, "alice")); err != nil {
			return err
		}
		return tx.InsertToken(usecase.Token{Hash: []byte("old"), Account: vaultsync.ID{1}, Kind: usecase.TokenAccount, Expires: 10})
	})
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.DeleteExpiredTokens(11); err != nil {
			return err
		}
		if _, ok, _ := tx.TokenByHash([]byte("old")); ok {
			t.Fatal("an expired token survived the sweep")
		}
		return nil
	})
}

func envelope(record byte, version uint64) vaultsync.Envelope {
	return vaultsync.Envelope{Vault: vaultsync.ID{50}, Record: vaultsync.ID{record}, Version: version, Suite: vaultsync.Suite1,
		WrappedDEK: make([]byte, 72), Body: bytes.Repeat([]byte{record}, 296), Author: vaultsync.ID{7}, SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64)}
}

func head(seq uint64) vaultsync.Head {
	return vaultsync.Head{Vault: vaultsync.ID{50}, Seq: seq, Commit: vaultsync.ID{byte(seq)}, Author: vaultsync.ID{7}, SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64)}
}

func TestVaultsMembersAndCommits(t *testing.T) {
	db, _ := openTemp(t)
	op := vaultsync.MemberOp{Vault: vaultsync.ID{50}, Seq: 1, Kind: vaultsync.MemberCreate, Author: vaultsync.ID{1}, AuthorDevice: vaultsync.ID{7}, Suite: vaultsync.Suite1, SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64)}
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.InsertVault(usecase.Vault{ID: vaultsync.ID{50}, Owner: vaultsync.ID{1}, Personal: true}); err != nil {
			return err
		}
		add := op
		add.Seq, add.Kind, add.Subject = 2, vaultsync.MemberAdd, vaultsync.ID{2}
		if err := tx.AppendMemberOps(vaultsync.ID{50}, []vaultsync.MemberOp{op, add}); err != nil {
			return err
		}
		if err := tx.ReplaceMembers(vaultsync.ID{50}, map[vaultsync.ID]vaultsync.Role{{1}: vaultsync.RoleOwner, {2}: vaultsync.RoleViewer}); err != nil {
			return err
		}
		if err := tx.InsertCommit(vaultsync.ID{50}, 1, vaultsync.Commit{Head: head(1), Records: []vaultsync.Envelope{envelope(1, 1), envelope(2, 1)}}, 1); err != nil {
			return err
		}
		if err := tx.InsertCommit(vaultsync.ID{50}, 2, vaultsync.Commit{Head: head(2), Records: []vaultsync.Envelope{envelope(1, 2)}}, 1); err != nil {
			return err
		}
		return tx.UpdateVaultHead(vaultsync.ID{50}, 2, 900)
	})
	tx(t, db, func(tx usecase.Tx) error {
		if role, _ := tx.MemberRole(vaultsync.ID{50}, vaultsync.ID{2}); role != vaultsync.RoleViewer {
			t.Fatalf("role = %d", role)
		}
		if role, _ := tx.MemberRole(vaultsync.ID{50}, vaultsync.ID{3}); role != vaultsync.RoleNone {
			t.Fatal("a non-member has a role")
		}
		if shares, _ := tx.SharesVault(vaultsync.ID{1}, vaultsync.ID{2}); !shares {
			t.Fatal("co-members do not share a vault")
		}
		if shares, _ := tx.SharesVault(vaultsync.ID{1}, vaultsync.ID{3}); shares {
			t.Fatal("a stranger shares a vault")
		}
		cur, err := tx.CurrentRecords(vaultsync.ID{50})
		if err != nil || len(cur) != 2 {
			t.Fatalf("current records = %d, %v", len(cur), err)
		}
		for _, e := range cur {
			if e.Record == (vaultsync.ID{1}) && e.Version != 2 {
				t.Fatalf("current version of record 1 = %d, want 2", e.Version)
			}
		}
		page, err := tx.Commits(vaultsync.ID{50}, 1, 10)
		if err != nil || len(page) != 1 || page[0].Seq != 2 || len(page[0].Commit.Records) != 1 {
			t.Fatalf("commits after 1 = %+v, %v", page, err)
		}
		if seq, ok, _ := tx.CommitSeqByID(vaultsync.ID{50}, vaultsync.ID{2}); !ok || seq != 2 {
			t.Fatalf("commit by id = %d, %v", seq, ok)
		}
		if h, ok, _ := tx.LatestHead(vaultsync.ID{50}); !ok || h.Seq != 2 {
			t.Fatalf("latest head = %+v, %v", h, ok)
		}
		v, _, _ := tx.VaultByID(vaultsync.ID{50})
		if v.Seq != 2 || v.Bytes != 900 || !v.Personal {
			t.Fatalf("vault = %+v", v)
		}
		ops, _ := tx.MemberOps(vaultsync.ID{50})
		if len(ops) != 2 || ops[0].Kind != vaultsync.MemberCreate {
			t.Fatalf("ops = %+v", ops)
		}
		vs, _ := tx.VaultsOf(vaultsync.ID{2})
		if len(vs) != 1 {
			t.Fatalf("vaults of a member = %d", len(vs))
		}
		return tx.SetVaultMembershipSeq(vaultsync.ID{50}, 1)
	})
	err := db.WithTx(ctx, func(tx usecase.Tx) error {
		return tx.InsertCommit(vaultsync.ID{50}, 3, vaultsync.Commit{Head: head(2)}, 1)
	})
	if err == nil {
		t.Fatal("a second commit with one commit id was stored")
	}
}

// A purged record's ciphertext must not survive in the database file (spec section 10.1, S3).
func TestPurgedVersionsLeaveTheFile(t *testing.T) {
	db, path := openTemp(t)
	marker := bytes.Repeat([]byte("EXHUME-ME-"), 30)
	e := envelope(1, 1)
	e.Body = marker
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.InsertVault(usecase.Vault{ID: vaultsync.ID{50}, Owner: vaultsync.ID{1}}); err != nil {
			return err
		}
		return tx.InsertCommit(vaultsync.ID{50}, 1, vaultsync.Commit{Head: head(1), Records: []vaultsync.Envelope{e, envelope(2, 1)}}, 1)
	})
	tx(t, db, func(tx usecase.Tx) error {
		seq, err := tx.DeleteRecords(vaultsync.ID{50}, []vaultsync.ID{{1}})
		if err != nil || seq != 1 {
			t.Fatalf("deleted through seq %d, %v; want 1", seq, err)
		}
		if err := tx.RecomputeBytes(vaultsync.ID{50}); err != nil {
			return err
		}
		v, _, _ := tx.VaultByID(vaultsync.ID{50})
		if v.Bytes != 72+296 {
			t.Fatalf("bytes = %d, want only the remaining record's ciphertext", v.Bytes)
		}
		left, _ := tx.RecordVersions(vaultsync.ID{50}, vaultsync.ID{2})
		if len(left) != 1 {
			t.Fatal("a purge deleted another record")
		}
		return nil
	})
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	needle := []byte(base64.StdEncoding.EncodeToString(marker))
	for _, p := range []string{path, path + "-wal"} {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if bytes.Contains(content, needle) || bytes.Contains(content, marker) {
			t.Fatalf("%s still contains a purged record's ciphertext", filepath.Base(p))
		}
	}
}

func TestDeleteSupersededKeepsTheLatestVersion(t *testing.T) {
	db, _ := openTemp(t)
	tx(t, db, func(tx usecase.Tx) error {
		if err := tx.InsertVault(usecase.Vault{ID: vaultsync.ID{50}, Owner: vaultsync.ID{1}}); err != nil {
			return err
		}
		if err := tx.InsertCommit(vaultsync.ID{50}, 1, vaultsync.Commit{Head: head(1), Records: []vaultsync.Envelope{envelope(1, 1)}}, 10); err != nil {
			return err
		}
		return tx.InsertCommit(vaultsync.ID{50}, 2, vaultsync.Commit{Head: head(2), Records: []vaultsync.Envelope{envelope(1, 2)}}, 20)
	})
	tx(t, db, func(tx usecase.Tx) error {
		if seq, _ := tx.DeleteSuperseded(vaultsync.ID{50}, 20); seq != 0 {
			t.Fatal("a version superseded at 20 was deleted by a cutoff of 20; the cutoff is exclusive")
		}
		if seq, _ := tx.DeleteSuperseded(vaultsync.ID{50}, 1000); seq != 1 {
			t.Fatalf("seq = %d, want 1: the superseded version lived in commit 1", seq)
		}
		left, _ := tx.RecordVersions(vaultsync.ID{50}, vaultsync.ID{1})
		if len(left) != 1 || left[0].Version != 2 {
			t.Fatalf("left = %+v; the latest version is never history", left)
		}
		if seq, _ := tx.DeleteEpochVersions(vaultsync.ID{50}, []uint32{0}); seq != 0 {
			t.Fatal("an epoch delete removed a latest version")
		}
		if err := tx.RaisePrunedSeq(vaultsync.ID{50}, 5); err != nil {
			return err
		}
		if err := tx.RaisePrunedSeq(vaultsync.ID{50}, 3); err != nil {
			return err
		}
		v, _, _ := tx.VaultByID(vaultsync.ID{50})
		if v.PrunedSeq != 5 {
			t.Fatalf("pruned seq = %d, want 5: it never moves back", v.PrunedSeq)
		}
		return nil
	})
}
