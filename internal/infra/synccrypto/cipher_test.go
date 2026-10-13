package synccrypto_test

import (
	"bytes"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

func TestCipherSealsIntoTheEnvelopeAndOpensFromIt(t *testing.T) {
	var c synctrust.RecordCipher = synccrypto.Cipher{}
	vk := bytes.Repeat([]byte{3}, 32)
	e := vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 4, Epoch: 5, Suite: vaultsync.Suite1}
	if err := c.Seal(vk, &e, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if len(e.WrappedDEK) == 0 || len(e.Body) == 0 {
		t.Fatal("Seal left the envelope empty")
	}
	got, err := c.Open(vk, &e)
	if err != nil || string(got) != "hello" {
		t.Fatalf("open = %q, %v", got, err)
	}
	moved := e
	moved.Version = 5
	if _, err := c.Open(vk, &moved); err == nil {
		t.Fatal("a body opened under another version; a replayed old body would pass as new")
	}
}
