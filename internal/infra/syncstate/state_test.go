package syncstate_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/syncstate"
)

var ctx = context.Background()

func openTemp(t *testing.T) (*syncstate.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := syncstate.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func envelope(vault, record byte, version uint64, body []byte) vaultsync.Envelope {
	if body == nil {
		body = bytes.Repeat([]byte{6}, 296)
	}
	return vaultsync.Envelope{Vault: vaultsync.ID{vault}, Record: vaultsync.ID{record}, Version: version, Epoch: 1,
		Suite: vaultsync.Suite1, WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: body, Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
}

func head(vault byte, seq uint64) vaultsync.Head {
	return vaultsync.Head{Vault: vaultsync.ID{vault}, Seq: seq, PrevHash: [32]byte{10}, MerkleRoot: [32]byte{11},
		CurrentEpoch: 1, Commit: vaultsync.ID{byte(seq), 15}, Author: vaultsync.ID{16}, ClientTimeUnixMilli: 17,
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{18}, 64)}
}

func entry(seq uint64) vaultsync.DeviceEntry {
	return vaultsync.DeviceEntry{Account: vaultsync.ID{19}, Seq: seq, Prev: [32]byte{21}, Op: vaultsync.OpAddDevice,
		Cert:   &vaultsync.DeviceCert{Device: vaultsync.ID{22}, SigningKey: bytes.Repeat([]byte{23}, 32), Name: "n", Platform: "p", CreatedUnixMilli: 24},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{25}, 64)}
}

func op(seq uint64) vaultsync.MemberOp {
	return vaultsync.MemberOp{Vault: vaultsync.ID{1}, Seq: seq, Prev: [32]byte{42}, Kind: vaultsync.MemberSetPolicy,
		Author: vaultsync.ID{43}, AuthorDevice: vaultsync.ID{44}, RetentionDays: 30, TimeUnixMilli: 51,
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{52}, 64)}
}

func TestOpenAndReopen(t *testing.T) {
	db, path := openTemp(t)
	if v, err := db.SchemaVersion(ctx); err != nil || v != 1 {
		t.Fatalf("schema version = %d, %v; want 1", v, err)
	}
	if on, err := db.SecureDelete(ctx); err != nil || !on {
		t.Fatalf("secure_delete = %v, %v", on, err)
	}
	_ = db.Close()
	again, err := syncstate.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	_ = again.Close()
}

func TestRefusesNewerStateWithoutTouchingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	raw, _ := sql.Open("sqlite", path)
	if _, err := raw.ExecContext(ctx, "CREATE TABLE future (x INTEGER); PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	before, _ := os.ReadFile(path)
	if _, err := syncstate.Open(ctx, path); !errors.Is(err, syncstate.ErrStateTooNew) {
		t.Fatalf("err = %v, want ErrStateTooNew", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("refusing newer sync state modified the file")
	}
}

func TestRefusesUnknownDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	raw, _ := sql.Open("sqlite", path)
	if _, err := raw.ExecContext(ctx, "CREATE TABLE notours (x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if _, err := syncstate.Open(ctx, path); !errors.Is(err, syncstate.ErrStateUnknown) {
		t.Fatalf("err = %v, want ErrStateUnknown", err)
	}
}

func TestAdvanceVault(t *testing.T) {
	db, _ := openTemp(t)
	if sv, err := db.LoadVault(ctx, vaultsync.ID{1}); err != nil || sv.Head != nil || len(sv.Records) != 0 {
		t.Fatalf("unknown vault = %+v, %v; want empty", sv, err)
	}
	if err := db.AdvanceVault(ctx, head(1, 1), []vaultsync.Envelope{envelope(1, 2, 1, nil), envelope(1, 3, 1, nil)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.AdvanceVault(ctx, head(1, 2), []vaultsync.Envelope{envelope(1, 2, 2, nil)}, nil); err != nil {
		t.Fatal(err)
	}
	sv, err := db.LoadVault(ctx, vaultsync.ID{1})
	if err != nil || sv.Head == nil || sv.Head.Seq != 2 || len(sv.Records) != 2 {
		t.Fatalf("loaded %+v, %v", sv, err)
	}
	for _, r := range sv.Records {
		if r.Record == (vaultsync.ID{2}) && r.Version != 2 {
			t.Fatal("a newer version did not replace the stored one")
		}
	}
	if other, _ := db.LoadVault(ctx, vaultsync.ID{9}); other.Head != nil {
		t.Fatal("one vault's head leaked into another")
	}
}

func TestAdvanceVaultRefusesToGoBack(t *testing.T) {
	db, _ := openTemp(t)
	if err := db.AdvanceVault(ctx, head(1, 3), nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{3, 2} {
		if err := db.AdvanceVault(ctx, head(1, seq), []vaultsync.Envelope{envelope(1, 2, 9, nil)}, nil); !errors.Is(err, synctrust.ErrStaleHead) {
			t.Errorf("head %d after 3: err = %v, want ErrStaleHead", seq, err)
		}
	}
	if sv, _ := db.LoadVault(ctx, vaultsync.ID{1}); len(sv.Records) != 0 {
		t.Fatal("a refused advance still stored its records")
	}
}

func TestAdvanceVaultRefusesForeignRecords(t *testing.T) {
	db, _ := openTemp(t)
	if err := db.AdvanceVault(ctx, head(1, 1), []vaultsync.Envelope{envelope(2, 2, 1, nil)}, nil); err == nil {
		t.Fatal("a record of vault 2 was stored under vault 1's head")
	}
}

func TestLogsMustBeContiguous(t *testing.T) {
	db, _ := openTemp(t)
	account := vaultsync.ID{19}
	if err := db.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry(2)}); !errors.Is(err, synctrust.ErrLogGap) {
		t.Fatalf("first entry at seq 2: %v, want ErrLogGap", err)
	}
	if err := db.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry(1), entry(2)}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry(4)}); !errors.Is(err, synctrust.ErrLogGap) {
		t.Fatalf("seq 4 after 2: %v, want ErrLogGap", err)
	}
	if err := db.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry(3), entry(5)}); !errors.Is(err, synctrust.ErrLogGap) {
		t.Fatalf("a gap inside one batch: %v, want ErrLogGap", err)
	}
	got, err := db.DeviceLog(ctx, account)
	if err != nil || len(got) != 2 || got[1].Seq != 2 {
		t.Fatalf("device log = %d entries, %v; a refused batch must store nothing", len(got), err)
	}
	if err := db.AppendMembership(ctx, []vaultsync.MemberOp{op(1), op(2)}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendMembership(ctx, []vaultsync.MemberOp{op(2)}); !errors.Is(err, synctrust.ErrLogGap) {
		t.Fatalf("membership replay: %v, want ErrLogGap", err)
	}
	if ops, _ := db.Membership(ctx, vaultsync.ID{1}); len(ops) != 2 || ops[0].RetentionDays != 30 {
		t.Fatalf("membership = %+v", ops)
	}
}

func TestOutbox(t *testing.T) {
	db, _ := openTemp(t)
	c := vaultsync.Commit{Head: head(1, 1), Records: []vaultsync.Envelope{envelope(1, 2, 1, nil)}}
	if got, err := db.Outbox(ctx, vaultsync.ID{1}); err != nil || got != nil {
		t.Fatalf("empty outbox = %v, %v", got, err)
	}
	if err := db.PutOutbox(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := db.PutOutbox(ctx, vaultsync.Commit{Head: head(1, 2)}); !errors.Is(err, synctrust.ErrOutboxBusy) {
		t.Fatalf("second pending commit: %v, want ErrOutboxBusy", err)
	}
	got, err := db.Outbox(ctx, vaultsync.ID{1})
	if err != nil || got == nil || got.Head.Commit != c.Head.Commit || len(got.Records) != 1 {
		t.Fatalf("outbox = %+v, %v", got, err)
	}
	if err := db.ClearOutbox(ctx, vaultsync.ID{1}, vaultsync.ID{99}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Outbox(ctx, vaultsync.ID{1}); got == nil {
		t.Fatal("clearing another commit id emptied the outbox; a stale acknowledgement must not drop a pending commit")
	}
	if err := db.ClearOutbox(ctx, vaultsync.ID{1}, c.Head.Commit); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Outbox(ctx, vaultsync.ID{1}); got != nil {
		t.Fatal("outbox not cleared")
	}
}

func TestQuarantine(t *testing.T) {
	db, _ := openTemp(t)
	item := synctrust.QuarantineItem{Envelope: envelope(1, 2, 1, nil), Reason: "unknown schema", AtUnixMilli: 5}
	if err := db.Quarantine(ctx, item); err != nil {
		t.Fatal(err)
	}
	newer := item
	newer.Envelope.Version = 2
	if err := db.Quarantine(ctx, newer); err != nil {
		t.Fatal(err)
	}
	got, err := db.Quarantined(ctx, vaultsync.ID{1})
	if err != nil || len(got) != 1 || got[0].Envelope.Version != 2 || got[0].Reason != "unknown schema" {
		t.Fatalf("quarantine = %+v, %v; a newer version replaces the older one", got, err)
	}
	held := synctrust.QuarantineItem{Envelope: envelope(1, 3, 1, nil), Reason: "weakens plugin trust", Pending: true}
	if err := db.Quarantine(ctx, held); err != nil {
		t.Fatal(err)
	}
	got, _ = db.Quarantined(ctx, vaultsync.ID{1})
	if len(got) != 2 || got[0].Pending || !got[1].Pending {
		t.Fatalf("quarantine = %+v; a held change must stay distinguishable from a refused one", got)
	}
	if err := db.Release(ctx, vaultsync.ID{1}, vaultsync.ID{3}); err != nil {
		t.Fatal(err)
	}
	if err := db.Release(ctx, vaultsync.ID{1}, vaultsync.ID{2}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Quarantined(ctx, vaultsync.ID{1}); len(got) != 0 {
		t.Fatal("release left the item")
	}
}

// Forgetting a vault - leaving sync, or a vault deleted on the server - must not leave its
// ciphertext readable in the file (spec §12.4, the same rule as the server).
func TestForgottenVaultLeavesTheFile(t *testing.T) {
	db, path := openTemp(t)
	marker := bytes.Repeat([]byte("EXHUME-ME-"), 30)
	_ = db.AdvanceVault(ctx, head(1, 1), []vaultsync.Envelope{envelope(1, 2, 1, marker[:296])}, nil)
	_ = db.Quarantine(ctx, synctrust.QuarantineItem{Envelope: envelope(1, 3, 1, marker[:296]), Reason: "x"})
	_ = db.PutOutbox(ctx, vaultsync.Commit{Head: head(1, 2), Records: []vaultsync.Envelope{envelope(1, 4, 1, marker[:296])}})
	if err := db.ForgetVault(ctx, vaultsync.ID{1}); err != nil {
		t.Fatal(err)
	}
	if sv, _ := db.LoadVault(ctx, vaultsync.ID{1}); sv.Head != nil || len(sv.Records) != 0 {
		t.Fatal("forgotten vault still loads")
	}
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for _, p := range []string{path, path + "-wal"} {
		content, err := os.ReadFile(p)
		// Envelopes are stored as JSON, where bytes are base64; look for both spellings, or the
		// check would pass on a file that holds every byte of the ciphertext.
		b64 := []byte(base64.StdEncoding.EncodeToString(marker[:30]))
		if err == nil && (bytes.Contains(content, marker[:40]) || bytes.Contains(content, b64)) {
			t.Fatalf("%s still holds a forgotten vault's ciphertext", filepath.Base(p))
		}
	}
}

// Turning sync off leaves nothing of the account behind: no team vault, no device log, no pending
// commit. The tables are read from the schema itself, so a table added later and missed by
// ForgetAll fails here rather than surviving every "disable".
func TestForgetAllLeavesNoRowInAnyTable(t *testing.T) {
	db, path := openTemp(t)
	for _, v := range []byte{1, 2} {
		if err := db.AdvanceVault(ctx, head(v, 1), []vaultsync.Envelope{envelope(v, 2, 1, nil)}, nil); err != nil {
			t.Fatal(err)
		}
		_ = db.Quarantine(ctx, synctrust.QuarantineItem{Envelope: envelope(v, 3, 1, nil), Reason: "x"})
		_ = db.PutOutbox(ctx, vaultsync.Commit{Head: head(v, 2), Records: []vaultsync.Envelope{envelope(v, 4, 1, nil)}})
	}
	if err := db.AppendDeviceLog(ctx, []vaultsync.DeviceEntry{entry(1)}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendMembership(ctx, []vaultsync.MemberOp{op(1)}); err != nil {
		t.Fatal(err)
	}
	if err := db.ForgetAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	rs, err := raw.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rs.Next() {
		var name string
		_ = rs.Scan(&name)
		tables = append(tables, name)
	}
	_ = rs.Close()
	if len(tables) < 6 {
		t.Fatalf("tables = %v; the schema lookup found too few to mean anything", tables)
	}
	for _, table := range tables {
		var n int
		if err := raw.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil { // #nosec G202 -- names from sqlite_master
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("table %s holds %d rows after ForgetAll; turning sync off must forget the whole account", table, n)
		}
	}
}

func TestImplementsThePort(t *testing.T) {
	var _ synctrust.StateStore = (*syncstate.DB)(nil)
}

func TestAdvanceVaultDeletesPurgedRecords(t *testing.T) {
	db, _ := openTemp(t)
	if err := db.AdvanceVault(ctx, head(1, 1), []vaultsync.Envelope{envelope(1, 2, 1, nil), envelope(1, 3, 1, nil)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.AdvanceVault(ctx, head(1, 2), nil, []vaultsync.ID{{2}}); err != nil {
		t.Fatal(err)
	}
	sv, _ := db.LoadVault(ctx, vaultsync.ID{1})
	if len(sv.Records) != 1 || sv.Records[0].Record != (vaultsync.ID{3}) {
		t.Fatalf("records = %+v; the purged record must leave the verified base", sv.Records)
	}
	if _, ok := sv.LandedAt[vaultsync.ID{2}]; ok {
		t.Fatal("a purged record kept its landing time")
	}
}

func TestLandedAtIsTheTimeOfTheHeadThatCarriedTheVersion(t *testing.T) {
	db, _ := openTemp(t)
	h1 := head(1, 1)
	h1.ClientTimeUnixMilli = 100
	h2 := head(1, 2)
	h2.ClientTimeUnixMilli = 250
	if err := db.AdvanceVault(ctx, h1, []vaultsync.Envelope{envelope(1, 2, 1, nil), envelope(1, 3, 1, nil)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.AdvanceVault(ctx, h2, []vaultsync.Envelope{envelope(1, 2, 2, nil)}, nil); err != nil {
		t.Fatal(err)
	}
	sv, _ := db.LoadVault(ctx, vaultsync.ID{1})
	if sv.LandedAt[vaultsync.ID{2}] != 250 || sv.LandedAt[vaultsync.ID{3}] != 100 {
		t.Fatalf("landed at = %v; want each record's latest version dated by the head that carried it", sv.LandedAt)
	}
}
