package synccrypto_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

var bundleAccount = vaultsync.ID{0xb0}

func exportKey(seed byte) []byte { return bytes.Repeat([]byte{seed}, 64) }

func sampleKeys() synccrypto.AccountKeys {
	return synccrypto.AccountKeys{IdentitySeed: bytes.Repeat([]byte{1}, 32), EncryptionSeed: bytes.Repeat([]byte{2}, 32)}
}

func TestBundleRoundTrip(t *testing.T) {
	sealed, err := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, sampleKeys())
	if err != nil {
		t.Fatal(err)
	}
	got, err := synccrypto.OpenBundle(exportKey(9), bundleAccount, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.IdentitySeed, sampleKeys().IdentitySeed) || !bytes.Equal(got.EncryptionSeed, sampleKeys().EncryptionSeed) {
		t.Fatalf("bundle round trip changed the keys: %+v", got)
	}
}

func TestBundleRefusesWrongExportKeyOrAccount(t *testing.T) {
	sealed, _ := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, sampleKeys())
	if _, err := synccrypto.OpenBundle(exportKey(8), bundleAccount, sealed); !errors.Is(err, synccrypto.ErrOpen) {
		t.Errorf("wrong export key (a wrong password): err = %v, want ErrOpen", err)
	}
	if _, err := synccrypto.OpenBundle(exportKey(9), vaultsync.ID{0xb1}, sealed); !errors.Is(err, synccrypto.ErrOpen) {
		t.Errorf("other account: err = %v, want ErrOpen; the server must not hand one account another's bundle", err)
	}
}

func TestBundleRefusesBitFlips(t *testing.T) {
	sealed, _ := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, sampleKeys())
	for i := range sealed {
		s := bytes.Clone(sealed)
		s[i] ^= 0x08
		if _, err := synccrypto.OpenBundle(exportKey(9), bundleAccount, s); !errors.Is(err, synccrypto.ErrOpen) {
			t.Fatalf("byte %d flipped still opened", i)
		}
	}
}

func TestSealBundleRefusesBadSeeds(t *testing.T) {
	bad := []synccrypto.AccountKeys{
		{IdentitySeed: make([]byte, 31), EncryptionSeed: make([]byte, 32)},
		{IdentitySeed: make([]byte, 32), EncryptionSeed: nil},
	}
	for i, keys := range bad {
		if _, err := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, keys); err == nil {
			t.Errorf("case %d: a malformed key set was sealed", i)
		}
	}
	if _, err := synccrypto.SealBundle(rand.Reader, nil, bundleAccount, sampleKeys()); err == nil {
		t.Error("an empty export key was accepted")
	}
}

func TestBundlesAreFreshlyNonced(t *testing.T) {
	a, _ := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, sampleKeys())
	b, _ := synccrypto.SealBundle(rand.Reader, exportKey(9), bundleAccount, sampleKeys())
	if bytes.Equal(a, b) {
		t.Fatal("two bundle seals are identical; the nonce is not fresh")
	}
}

func FuzzOpenBundle(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, 200))
	f.Fuzz(func(t *testing.T, s []byte) {
		if _, err := synccrypto.OpenBundle(exportKey(9), bundleAccount, s); err == nil {
			t.Fatal("arbitrary bytes opened as a bundle")
		}
	})
}
