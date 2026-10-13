package synctrust_test

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/synctrust"
)

func TestVaultMetaRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, synctrust.NamingKeySize)
	body, err := synctrust.NewVaultMetaBody(key)
	if err != nil || body.Type != synctrust.TypeVaultMeta || body.Schema != 1 {
		t.Fatalf("body = %+v, %v", body, err)
	}
	got, err := synctrust.VaultMetaNamingKey(body)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("naming key = %x, %v", got, err)
	}
}

func TestVaultMetaRefusals(t *testing.T) {
	if _, err := synctrust.NewVaultMetaBody(make([]byte, 31)); !errors.Is(err, synctrust.ErrNamingKey) {
		t.Errorf("short key: %v", err)
	}
	for name, b := range map[string]synctrust.Body{
		"wrong type": {Type: synctrust.TypeFolder, Schema: 1, Data: []byte(`{"namingKey":"` + string(bytes.Repeat([]byte("A"), 43)) + `="}`)},
		"short key":  {Type: synctrust.TypeVaultMeta, Schema: 1, Data: []byte(`{"namingKey":"AAAA"}`)},
		"extra":      {Type: synctrust.TypeVaultMeta, Schema: 1, Data: []byte(`{"namingKey":"AAAA","x":1}`)},
	} {
		if _, err := synctrust.VaultMetaNamingKey(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
