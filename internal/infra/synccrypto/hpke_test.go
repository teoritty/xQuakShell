package synccrypto_test

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

var (
	wrapVault  = vaultsync.ID{0x11}
	wrapMember = vaultsync.ID{0x22}
)

func TestEncryptionKeySizesMatchSuite1(t *testing.T) {
	p, _ := vaultsync.LookupSuite(vaultsync.Suite1, vaultsync.MinAcceptedSuite)
	seed, pub, err := synccrypto.GenerateEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != p.EncryptionKeySize || len(seed) != 32 {
		t.Fatalf("public %d, seed %d; suite 1 fixes the public key at %d bytes", len(pub), len(seed), p.EncryptionKeySize)
	}
	wrapped, err := synccrypto.WrapVaultKey(pub, wrapVault, 1, wrapMember, vaultKey(5))
	if err != nil {
		t.Fatal(err)
	}
	if len(wrapped) != p.VKWrapSize {
		t.Fatalf("wrap is %d bytes, suite 1 fixes %d; the membership log rejects any other size", len(wrapped), p.VKWrapSize)
	}
}

func TestWrapUnwrapRoundTrip(t *testing.T) {
	seed, pub, _ := synccrypto.GenerateEncryptionKey()
	wrapped, err := synccrypto.WrapVaultKey(pub, wrapVault, 7, wrapMember, vaultKey(5))
	if err != nil {
		t.Fatal(err)
	}
	vk, err := synccrypto.UnwrapVaultKey(seed, wrapVault, 7, wrapMember, wrapped)
	if err != nil || !bytes.Equal(vk, vaultKey(5)) {
		t.Fatalf("unwrap = %x, %v", vk, err)
	}
}

func TestUnwrapRefusesOtherVaultEpochOrMember(t *testing.T) {
	seed, pub, _ := synccrypto.GenerateEncryptionKey()
	wrapped, _ := synccrypto.WrapVaultKey(pub, wrapVault, 7, wrapMember, vaultKey(5))
	cases := map[string]func() ([]byte, error){
		"vault": func() ([]byte, error) {
			return synccrypto.UnwrapVaultKey(seed, vaultsync.ID{0x99}, 7, wrapMember, wrapped)
		},
		"epoch": func() ([]byte, error) { return synccrypto.UnwrapVaultKey(seed, wrapVault, 8, wrapMember, wrapped) },
		"member": func() ([]byte, error) {
			return synccrypto.UnwrapVaultKey(seed, wrapVault, 7, vaultsync.ID{0x99}, wrapped)
		},
	}
	for name, unwrap := range cases {
		if _, err := unwrap(); !errors.Is(err, synccrypto.ErrOpen) {
			t.Errorf("other %s: err = %v, want ErrOpen; a wrap must not be replayable elsewhere", name, err)
		}
	}
}

func TestUnwrapRefusesOtherRecipient(t *testing.T) {
	_, pub, _ := synccrypto.GenerateEncryptionKey()
	otherSeed, _, _ := synccrypto.GenerateEncryptionKey()
	wrapped, _ := synccrypto.WrapVaultKey(pub, wrapVault, 1, wrapMember, vaultKey(5))
	if _, err := synccrypto.UnwrapVaultKey(otherSeed, wrapVault, 1, wrapMember, wrapped); !errors.Is(err, synccrypto.ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func TestUnwrapRefusesBitFlips(t *testing.T) {
	seed, pub, _ := synccrypto.GenerateEncryptionKey()
	wrapped, _ := synccrypto.WrapVaultKey(pub, wrapVault, 1, wrapMember, vaultKey(5))
	for i := 0; i < len(wrapped)*8; i += 61 {
		w := bytes.Clone(wrapped)
		w[i/8] ^= 1 << (i % 8)
		if _, err := synccrypto.UnwrapVaultKey(seed, wrapVault, 1, wrapMember, w); !errors.Is(err, synccrypto.ErrOpen) {
			t.Fatalf("bit %d flipped still unwrapped", i)
		}
	}
}

func TestUnwrapRefusesWrongSizes(t *testing.T) {
	seed, pub, _ := synccrypto.GenerateEncryptionKey()
	wrapped, _ := synccrypto.WrapVaultKey(pub, wrapVault, 1, wrapMember, vaultKey(5))
	for _, w := range [][]byte{nil, wrapped[:len(wrapped)-1], append(bytes.Clone(wrapped), 0)} {
		if _, err := synccrypto.UnwrapVaultKey(seed, wrapVault, 1, wrapMember, w); !errors.Is(err, synccrypto.ErrOpen) {
			t.Errorf("a %d-byte wrap: err = %v, want ErrOpen", len(w), err)
		}
	}
	if _, err := synccrypto.UnwrapVaultKey(seed[:31], wrapVault, 1, wrapMember, wrapped); !errors.Is(err, synccrypto.ErrOpen) {
		t.Errorf("a 31-byte seed: err = %v, want ErrOpen", err)
	}
}

func TestWrapRefusesMalformedPublicKey(t *testing.T) {
	_, pub, _ := synccrypto.GenerateEncryptionKey()
	if _, err := synccrypto.WrapVaultKey(pub[:1215], wrapVault, 1, wrapMember, vaultKey(5)); err == nil {
		t.Error("a 1215-byte public key was accepted")
	}
	junk := make([]byte, len(pub))
	for i := range junk {
		junk[i] = 0xff
	}
	if _, err := synccrypto.WrapVaultKey(junk, wrapVault, 1, wrapMember, vaultKey(5)); err == nil {
		t.Error("an all-0xff public key was accepted; ML-KEM rejects encapsulation keys with out-of-range coefficients")
	}
}

func TestWrapRefusesBadVaultKeySize(t *testing.T) {
	_, pub, _ := synccrypto.GenerateEncryptionKey()
	if _, err := synccrypto.WrapVaultKey(pub, wrapVault, 1, wrapMember, make([]byte, 31)); err == nil {
		t.Fatal("a 31-byte vault key was wrapped")
	}
}

func TestEncryptionKeysAreFresh(t *testing.T) {
	a, _, _ := synccrypto.GenerateEncryptionKey()
	b, _, _ := synccrypto.GenerateEncryptionKey()
	if bytes.Equal(a, b) {
		t.Fatal("two generated encryption keys are equal")
	}
}

func FuzzUnwrapVaultKey(f *testing.F) {
	seed := bytes.Repeat([]byte{3}, 32)
	f.Add(make([]byte, 1168))
	f.Fuzz(func(t *testing.T, w []byte) {
		if _, err := synccrypto.UnwrapVaultKey(seed, wrapVault, 1, wrapMember, w); err == nil {
			t.Fatal("arbitrary bytes unwrapped as a vault key")
		}
	})
}
