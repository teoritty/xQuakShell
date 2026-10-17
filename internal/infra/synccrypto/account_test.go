package synccrypto_test

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

func TestAccountAdapterRoundTrips(t *testing.T) {
	var c synctrust.AccountCrypto = synccrypto.Account{}
	seed, pub, err := c.GenerateEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	derived, err := c.EncryptionPublic(seed)
	if err != nil || !bytes.Equal(derived, pub) {
		t.Fatalf("public key from seed = %x, %v; a device checking its bundle would refuse its own keys", derived, err)
	}
	if _, err := c.EncryptionPublic(seed[:31]); err == nil {
		t.Fatal("a short seed produced a public key")
	}
	_, idPriv, _ := ed25519.GenerateKey(nil)
	export := bytes.Repeat([]byte{1}, 64)
	sealed, err := c.SealBundle(export, vaultsync.ID{1}, idPriv.Seed(), seed)
	if err != nil {
		t.Fatal(err)
	}
	idSeed, encSeed, err := c.OpenBundle(export, vaultsync.ID{1}, sealed)
	if err != nil || !bytes.Equal(idSeed, idPriv.Seed()) || !bytes.Equal(encSeed, seed) {
		t.Fatalf("bundle round trip failed: %v", err)
	}
	vk := bytes.Repeat([]byte{9}, 32)
	wrapped, err := c.WrapVaultKey(pub, vaultsync.ID{2}, 0, vaultsync.ID{1}, vk)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.UnwrapVaultKey(seed, vaultsync.ID{2}, 0, vaultsync.ID{1}, wrapped)
	if err != nil || !bytes.Equal(got, vk) {
		t.Fatalf("unwrap = %x, %v", got, err)
	}
}
