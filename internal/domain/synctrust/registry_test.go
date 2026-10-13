package synctrust_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var namingKey = bytes.Repeat([]byte{0x4e}, synctrust.NamingKeySize)

func TestRecordIDIsStableAndDistinct(t *testing.T) {
	a, err := synctrust.RecordID(namingKey, synctrust.TypeConnection, "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := synctrust.RecordID(namingKey, synctrust.TypeConnection, "conn-1")
	if a != again {
		t.Fatal("one natural key named two records; two devices would create duplicates")
	}
	for name, other := range map[string]func() (vaultsync.ID, error){
		"other key":  func() (vaultsync.ID, error) { return synctrust.RecordID(namingKey, synctrust.TypeConnection, "conn-2") },
		"other type": func() (vaultsync.ID, error) { return synctrust.RecordID(namingKey, synctrust.TypeFolder, "conn-1") },
		"other naming key": func() (vaultsync.ID, error) {
			return synctrust.RecordID(bytes.Repeat([]byte{1}, synctrust.NamingKeySize), synctrust.TypeConnection, "conn-1")
		},
		"boundary shift": func() (vaultsync.ID, error) { return synctrust.RecordID(namingKey, "connectio", "nconn-1") },
	} {
		id, _ := other()
		if id == a {
			t.Errorf("%s produced the same record id", name)
		}
	}
}

func TestRecordIDNeedsANamingKey(t *testing.T) {
	if _, err := synctrust.RecordID(namingKey[:31], synctrust.TypeFolder, "x"); !errors.Is(err, synctrust.ErrNamingKey) {
		t.Fatalf("err = %v, want ErrNamingKey; an unkeyed name lets the server test guesses like a host name", err)
	}
}

func TestVaultMetaIDIsPerVault(t *testing.T) {
	a := synctrust.VaultMetaID(vaultsync.ID{1})
	if a == synctrust.VaultMetaID(vaultsync.ID{2}) || a.IsZero() {
		t.Fatal("vault-meta ids collide across vaults or are zero")
	}
	if a != synctrust.VaultMetaID(vaultsync.ID{1}) {
		t.Fatal("vault-meta id is not stable; a new device could not find the naming key")
	}
}

func TestBodyRoundTrip(t *testing.T) {
	in := synctrust.Body{Type: synctrust.TypeFolder, Schema: 1, Data: json.RawMessage(`{"id":"f1","name":"Prod"}`)}
	raw, err := synctrust.EncodeBody(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := synctrust.DecodeBody(raw)
	if err != nil || out.Type != in.Type || out.Schema != 1 || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("round trip = %+v, %v", out, err)
	}
}

func TestDecodeBodyRefusals(t *testing.T) {
	for name, raw := range map[string]string{
		"no type":       `{"schema":1,"data":{}}`,
		"zero schema":   `{"type":"folder","schema":0,"data":{}}`,
		"no data":       `{"type":"folder","schema":1}`,
		"array data":    `{"type":"folder","schema":1,"data":[1]}`,
		"unknown field": `{"type":"folder","schema":1,"data":{},"x":1}`,
		"trailing":      `{"type":"folder","schema":1,"data":{}} {}`,
		"not json":      `nope`,
	} {
		if _, err := synctrust.DecodeBody([]byte(raw)); !errors.Is(err, synctrust.ErrBody) {
			t.Errorf("%s: err = %v, want ErrBody", name, err)
		}
	}
}

func TestKnownSchemasCoverEveryType(t *testing.T) {
	known := synctrust.KnownSchemas()
	for _, ty := range []synctrust.RecordType{synctrust.TypeVaultMeta, synctrust.TypeFolder, synctrust.TypeConnection,
		synctrust.TypeIdentity, synctrust.TypePassword, synctrust.TypePluginSecret, synctrust.TypeKnownHost,
		synctrust.TypePeerTrust, synctrust.TypeConflict, synctrust.SettingsType("terminal")} {
		if known[string(ty)] == 0 {
			t.Errorf("%s has no schema version; records of it would be treated as an unknown type", ty)
		}
	}
	if known["settings.nonexistent"] != 0 {
		t.Error("a settings section that does not exist has a schema")
	}
}

func FuzzDecodeBody(f *testing.F) {
	f.Add([]byte(`{"type":"folder","schema":1,"data":{}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		b, err := synctrust.DecodeBody(raw)
		if err != nil {
			return
		}
		if b.Type == "" || b.Schema == 0 || len(b.Data) == 0 {
			t.Fatalf("decoded an incomplete body: %+v", b)
		}
	})
}
