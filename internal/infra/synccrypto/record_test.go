package synccrypto_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

func vaultKey(seed byte) []byte { return bytes.Repeat([]byte{seed}, 32) }

func coords() synccrypto.RecordCoords {
	return synccrypto.RecordCoords{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 4, Suite: vaultsync.Suite1}
}

func TestSealOpenRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 300, 70000} {
		plain := bytes.Repeat([]byte{0x42}, n)
		wrapped, body, err := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), plain)
		if err != nil {
			t.Fatal(err)
		}
		if len(wrapped) != 72 || len(body) != vaultsync.PaddedSize(n)+40 {
			t.Fatalf("n=%d: wrapped %d body %d; sizes must match what vaultsync.Envelope validates", n, len(wrapped), len(body))
		}
		got, err := synccrypto.OpenRecord(vaultKey(1), coords(), wrapped, body)
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("n=%d: open = %d bytes, %v", n, len(got), err)
		}
	}
}

func TestSealedRecordPassesEnvelopeValidation(t *testing.T) {
	wrapped, body, err := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	e := &vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 4, Suite: vaultsync.Suite1,
		WrappedDEK: wrapped, Body: body, Author: vaultsync.ID{9}, SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64)}
	if err := e.Validate(); err != nil {
		t.Fatalf("a freshly sealed record fails envelope validation: %v", err)
	}
}

func TestOpenRefusesEveryOtherCoordinate(t *testing.T) {
	wrapped, body, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("secret"))
	for name, mutate := range map[string]func(*synccrypto.RecordCoords){
		"vault":   func(c *synccrypto.RecordCoords) { c.Vault[0] ^= 1 },
		"record":  func(c *synccrypto.RecordCoords) { c.Record[0] ^= 1 },
		"version": func(c *synccrypto.RecordCoords) { c.Version++ },
		"epoch":   func(c *synccrypto.RecordCoords) { c.Epoch++ },
		"suite":   func(c *synccrypto.RecordCoords) { c.Suite++ },
	} {
		c := coords()
		mutate(&c)
		if _, err := synccrypto.OpenRecord(vaultKey(1), c, wrapped, body); !errors.Is(err, synccrypto.ErrOpen) {
			t.Errorf("%s changed: err = %v, want ErrOpen; a record moved by the server must not open", name, err)
		}
	}
}

func TestOpenRefusesOtherVaultKey(t *testing.T) {
	wrapped, body, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("secret"))
	if _, err := synccrypto.OpenRecord(vaultKey(2), coords(), wrapped, body); !errors.Is(err, synccrypto.ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func TestOpenRefusesAnyBitFlip(t *testing.T) {
	wrapped, body, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("secret"))
	for i := range len(wrapped) * 8 {
		w := bytes.Clone(wrapped)
		w[i/8] ^= 1 << (i % 8)
		if _, err := synccrypto.OpenRecord(vaultKey(1), coords(), w, body); !errors.Is(err, synccrypto.ErrOpen) {
			t.Fatalf("wrapped key bit %d flipped still opened", i)
		}
	}
	for i := range body {
		b := bytes.Clone(body)
		b[i] ^= 0x10
		if _, err := synccrypto.OpenRecord(vaultKey(1), coords(), wrapped, b); !errors.Is(err, synccrypto.ErrOpen) {
			t.Fatalf("body byte %d flipped still opened", i)
		}
	}
}

func TestFreshDEKPerSeal(t *testing.T) {
	w1, b1, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("same"))
	w2, b2, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("same"))
	if bytes.Equal(w1, w2) || bytes.Equal(b1, b2) {
		t.Fatal("two seals of one plaintext are identical; data keys or nonces are being reused")
	}
}

func TestSealRefusesBadVaultKeySize(t *testing.T) {
	for _, n := range []int{0, 31, 33} {
		if _, _, err := synccrypto.SealRecord(rand.Reader, make([]byte, n), coords(), []byte("x")); err == nil {
			t.Errorf("SealRecord accepted a %d-byte vault key", n)
		}
	}
}

func TestSealRefusesShortRandomness(t *testing.T) {
	if _, _, err := synccrypto.SealRecord(bytes.NewReader(make([]byte, 10)), vaultKey(1), coords(), []byte("x")); err == nil {
		t.Fatal("SealRecord succeeded with 10 bytes of randomness for a key and two nonces")
	}
}

func TestOpenRefusesUnknownSuite(t *testing.T) {
	wrapped, body, _ := synccrypto.SealRecord(rand.Reader, vaultKey(1), coords(), []byte("x"))
	c := coords()
	c.Suite = 99
	if _, err := synccrypto.OpenRecord(vaultKey(1), c, wrapped, body); !errors.Is(err, synccrypto.ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func FuzzOpenRecord(f *testing.F) {
	// The seed is sealed from fixed randomness: under -fuzz the corpus is built in one process and
	// replayed in others, and a random seal would differ between them.
	fixed := bytes.NewReader(bytes.Repeat([]byte{7}, 32+24+24))
	wrapped, body, _ := synccrypto.SealRecord(fixed, vaultKey(1), coords(), []byte("seed"))
	f.Add(wrapped, body)
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, w, b []byte) {
		if bytes.Equal(w, wrapped) && bytes.Equal(b, body) {
			return
		}
		if _, err := synccrypto.OpenRecord(vaultKey(1), coords(), w, b); err == nil {
			t.Fatal("arbitrary bytes opened as a record")
		}
	})
}
