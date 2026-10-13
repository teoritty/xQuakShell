package syncwire_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := syncwire.Decode(bytes.NewReader(raw), 1<<20, &out); err != nil {
		t.Fatalf("decode of our own encoding failed: %v\n%s", err, raw)
	}
	return out
}

func sampleEnvelope() vaultsync.Envelope {
	return vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 4, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: bytes.Repeat([]byte{6}, 296), Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
}

func sampleHead() vaultsync.Head {
	return vaultsync.Head{Vault: vaultsync.ID{1}, Seq: 9, PrevHash: [32]byte{10}, MerkleRoot: [32]byte{11}, CurrentEpoch: 12,
		MembershipSeq: 13, MembershipHead: [32]byte{14}, Commit: vaultsync.ID{15}, Author: vaultsync.ID{16},
		ClientTimeUnixMilli: 17, SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{18}, 64)}
}

func sampleEntry() vaultsync.DeviceEntry {
	return vaultsync.DeviceEntry{Account: vaultsync.ID{19}, Seq: 20, Prev: [32]byte{21}, Op: vaultsync.OpAddDevice,
		Cert:   &vaultsync.DeviceCert{Device: vaultsync.ID{22}, SigningKey: bytes.Repeat([]byte{23}, 32), Name: "n", Platform: "p", CreatedUnixMilli: 24},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{25}, 64)}
}

func sampleSuccession() vaultsync.DeviceEntry {
	return vaultsync.DeviceEntry{Account: vaultsync.ID{19}, Seq: 21, Prev: [32]byte{21}, Op: vaultsync.OpSuccession,
		Revoked: vaultsync.ID{30}, NewSuite: vaultsync.Suite1, NewIdentityKey: bytes.Repeat([]byte{31}, 32),
		NewEncryptionKey: bytes.Repeat([]byte{32}, 1216), SigAlg: vaultsync.SigEd25519,
		Sig: bytes.Repeat([]byte{33}, 64), SuccessorSig: bytes.Repeat([]byte{34}, 64)}
}

func sampleOp() vaultsync.MemberOp {
	return vaultsync.MemberOp{Vault: vaultsync.ID{40}, Seq: 41, Prev: [32]byte{42}, Kind: vaultsync.MemberRotateEpoch,
		Author: vaultsync.ID{43}, AuthorDevice: vaultsync.ID{44}, Subject: vaultsync.ID{45}, Role: vaultsync.RoleEditor,
		Verified: true, Fingerprint: [32]byte{46}, Suite: vaultsync.Suite1, Epoch: 47,
		Wraps:          []vaultsync.KeyWrap{{Member: vaultsync.ID{48}, Ciphertext: bytes.Repeat([]byte{49}, 1168)}},
		RevokedDevices: []vaultsync.ID{{50}}, RetentionDays: 30, RequireVerified: true, TimeUnixMilli: 51,
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{52}, 64)}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	want := sampleEnvelope()
	got, err := roundTrip(t, syncwire.EnvelopeFrom(&want)).ToDomain()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("envelope round trip: %+v, %v", got, err)
	}
	tomb := vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 4, Suite: vaultsync.Suite1, Deleted: true,
		Author: vaultsync.ID{7}, SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	gotTomb, err := roundTrip(t, syncwire.EnvelopeFrom(&tomb)).ToDomain()
	if err != nil || !gotTomb.Deleted || len(gotTomb.Body) != 0 || len(gotTomb.WrappedDEK) != 0 {
		t.Fatalf("tombstone round trip: %+v, %v", gotTomb, err)
	}
}

func TestHeadRoundTrip(t *testing.T) {
	want := sampleHead()
	got, err := roundTrip(t, syncwire.HeadFrom(&want)).ToDomain()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("head round trip: %+v, %v", got, err)
	}
}

func TestDeviceEntryRoundTrip(t *testing.T) {
	for _, want := range []vaultsync.DeviceEntry{sampleEntry(), sampleSuccession()} {
		got, err := roundTrip(t, syncwire.DeviceEntryFrom(&want)).ToDomain()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("device entry round trip:\n got %+v\nwant %+v\n%v", got, want, err)
		}
	}
}

func TestMemberOpRoundTrip(t *testing.T) {
	want := sampleOp()
	got, err := roundTrip(t, syncwire.MemberOpFrom(&want)).ToDomain()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("member op round trip:\n got %+v\nwant %+v\n%v", got, want, err)
	}
}

func TestRoundTripPreservesSignatures(t *testing.T) {
	e := sampleEnvelope()
	back, _ := roundTrip(t, syncwire.EnvelopeFrom(&e)).ToDomain()
	a, _ := e.SigningBytes()
	b, _ := back.SigningBytes()
	if !bytes.Equal(a, b) {
		t.Fatal("signing bytes differ after a wire round trip; every signature would fail on the other side")
	}
}

func TestDecodeRefusesUnknownFields(t *testing.T) {
	var h syncwire.Hello
	err := syncwire.Decode(strings.NewReader(`{"clientProtocol":{"major":1},"suite":1,"extra":true}`), 1024, &h)
	if !errors.Is(err, syncwire.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed for an unknown field", err)
	}
}

func TestDecodeRefusesTrailingValue(t *testing.T) {
	var h syncwire.Hello
	err := syncwire.Decode(strings.NewReader(`{"suite":1} {"suite":2}`), 1024, &h)
	if !errors.Is(err, syncwire.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed for a second JSON value", err)
	}
}

func TestDecodeRefusesOversize(t *testing.T) {
	var h syncwire.Hello
	body := `{"suite":1,"clientProtocol":{"major":1}}` + strings.Repeat(" ", 2000)
	if err := syncwire.Decode(strings.NewReader(body), 64, &h); !errors.Is(err, syncwire.ErrMalformed) || !errors.Is(err, syncwire.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrMalformed and ErrTooLarge for a body over the limit", err)
	}
	big := `{"suite":` + strings.Repeat("1", 200) + `}`
	if err := syncwire.Decode(strings.NewReader(big), 64, &h); !errors.Is(err, syncwire.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge for a value cut off by the limit", err)
	}
}

func TestToDomainRefusesBadIDsAndHashes(t *testing.T) {
	e := sampleEnvelope()
	d := syncwire.EnvelopeFrom(&e)
	d.Vault = "AB" + d.Vault[2:]
	if _, err := d.ToDomain(); !errors.Is(err, syncwire.ErrMalformed) {
		t.Errorf("upper-case id: err = %v", err)
	}
	h := sampleHead()
	hd := syncwire.HeadFrom(&h)
	hd.PrevHash = hd.PrevHash[:31]
	if _, err := hd.ToDomain(); !errors.Is(err, syncwire.ErrMalformed) {
		t.Errorf("31-byte hash: err = %v", err)
	}
	op := sampleOp()
	od := syncwire.MemberOpFrom(&op)
	od.RevokedDevices = []string{"nothex"}
	if _, err := od.ToDomain(); !errors.Is(err, syncwire.ErrMalformed) {
		t.Errorf("bad revoked device id: err = %v", err)
	}
	ent := sampleEntry()
	ed := syncwire.DeviceEntryFrom(&ent)
	ed.Cert.Device = ""
	if _, err := ed.ToDomain(); !errors.Is(err, syncwire.ErrMalformed) {
		t.Errorf("certificate without device id: err = %v", err)
	}
}

func TestOptionalIDsStayZero(t *testing.T) {
	ent := sampleEntry()
	d := syncwire.DeviceEntryFrom(&ent)
	if d.Revoked != "" {
		t.Fatalf("a zero id encoded as %q; absent ids travel as empty strings", d.Revoked)
	}
}

func TestErrorCodesAreClosed(t *testing.T) {
	for _, c := range syncwire.ErrorCodes() {
		if c == "" || strings.ContainsAny(c, " :") {
			t.Fatalf("error code %q is not a bare token; error bodies must never carry free text", c)
		}
	}
	if !syncwire.IsErrorCode("not_found") || syncwire.IsErrorCode("database is locked") {
		t.Fatal("IsErrorCode accepts something outside the closed list")
	}
}

func FuzzDecodeCommit(f *testing.F) {
	e := sampleEnvelope()
	h := sampleHead()
	seed, _ := json.Marshal(syncwire.Commit{ExpectSeq: 1, Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&e)}, Head: syncwire.HeadFrom(&h)})
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var c syncwire.Commit
		if syncwire.Decode(bytes.NewReader(raw), 1<<20, &c) != nil {
			return
		}
		_, _ = c.Head.ToDomain()
		for _, r := range c.Records {
			_, _ = r.ToDomain()
		}
	})
}

// Every identifier and hash field, corrupted on its own, must be refused: a field that slips
// through unchecked becomes a zero id or a truncated hash on the other side.
func TestEveryFieldIsChecked(t *testing.T) {
	const bad = "zz"
	short := []byte{1}
	e := sampleEnvelope()
	h := sampleHead()
	ent := sampleEntry()
	succ := sampleSuccession()
	op := sampleOp()
	cases := map[string]func() error{
		"envelope.record": func() error { d := syncwire.EnvelopeFrom(&e); d.Record = bad; _, err := d.ToDomain(); return err },
		"envelope.author": func() error { d := syncwire.EnvelopeFrom(&e); d.Author = bad; _, err := d.ToDomain(); return err },
		"head.vault":      func() error { d := syncwire.HeadFrom(&h); d.Vault = bad; _, err := d.ToDomain(); return err },
		"head.commit":     func() error { d := syncwire.HeadFrom(&h); d.Commit = bad; _, err := d.ToDomain(); return err },
		"head.author":     func() error { d := syncwire.HeadFrom(&h); d.Author = bad; _, err := d.ToDomain(); return err },
		"head.root":       func() error { d := syncwire.HeadFrom(&h); d.MerkleRoot = short; _, err := d.ToDomain(); return err },
		"head.membership": func() error { d := syncwire.HeadFrom(&h); d.MembershipHead = short; _, err := d.ToDomain(); return err },
		"entry.account":   func() error { d := syncwire.DeviceEntryFrom(&ent); d.Account = bad; _, err := d.ToDomain(); return err },
		"entry.prev":      func() error { d := syncwire.DeviceEntryFrom(&ent); d.Prev = short; _, err := d.ToDomain(); return err },
		"entry.revoked": func() error {
			d := syncwire.DeviceEntryFrom(&succ)
			d.Revoked = bad
			_, err := d.ToDomain()
			return err
		},
		"op.vault":  func() error { d := syncwire.MemberOpFrom(&op); d.Vault = bad; _, err := d.ToDomain(); return err },
		"op.author": func() error { d := syncwire.MemberOpFrom(&op); d.Author = bad; _, err := d.ToDomain(); return err },
		"op.device": func() error {
			d := syncwire.MemberOpFrom(&op)
			d.AuthorDevice = bad
			_, err := d.ToDomain()
			return err
		},
		"op.subject": func() error { d := syncwire.MemberOpFrom(&op); d.Subject = bad; _, err := d.ToDomain(); return err },
		"op.prev":    func() error { d := syncwire.MemberOpFrom(&op); d.Prev = short; _, err := d.ToDomain(); return err },
		"op.fingerprint": func() error {
			d := syncwire.MemberOpFrom(&op)
			d.Fingerprint = short
			_, err := d.ToDomain()
			return err
		},
		"op.wrap member": func() error {
			d := syncwire.MemberOpFrom(&op)
			d.Wraps[0].Member = bad
			_, err := d.ToDomain()
			return err
		},
	}
	for name, run := range cases {
		if err := run(); !errors.Is(err, syncwire.ErrMalformed) {
			t.Errorf("%s corrupted: err = %v, want ErrMalformed", name, err)
		}
	}
}

func TestIDHelpers(t *testing.T) {
	id := vaultsync.ID{0xab, 0xcd}
	got, err := syncwire.ParseID(syncwire.HexID(id))
	if err != nil || got != id {
		t.Fatalf("path id round trip = %v, %v", got, err)
	}
	if _, err := syncwire.ParseID("AB"); !errors.Is(err, syncwire.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if body := (syncwire.ErrorBody{Error: syncwire.CodeNotFound}); !syncwire.IsErrorCode(body.Error) {
		t.Fatal("the not_found body is not a known code")
	}
}
