package synccrypto_test

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

func TestSignVerifiesWithVaultsync(t *testing.T) {
	seed, pub, err := synccrypto.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) != 32 || len(pub) != 32 {
		t.Fatalf("seed %d, public %d; Ed25519 keys are 32 bytes", len(seed), len(pub))
	}
	sig, err := synccrypto.Sign(seed, []byte("message"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vaultsync.VerifySignature(vaultsync.SigEd25519, pub, []byte("message"), sig); err != nil {
		t.Fatalf("a signature made here fails the protocol's verifier: %v", err)
	}
	if err := vaultsync.VerifySignature(vaultsync.SigEd25519, pub, []byte("other"), sig); !errors.Is(err, vaultsync.ErrBadSignature) {
		t.Fatalf("signature verified for another message: %v", err)
	}
}

func TestSignRefusesBadSeedSize(t *testing.T) {
	for _, n := range []int{0, 31, 33, 64} {
		if _, err := synccrypto.Sign(make([]byte, n), []byte("m")); err == nil {
			t.Errorf("Sign accepted a %d-byte seed", n)
		}
	}
}

func TestSignedEnvelopeVerifies(t *testing.T) {
	seed, pub, _ := synccrypto.GenerateSigningKey()
	e := &vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 1, Suite: vaultsync.Suite1,
		Deleted: true, Author: vaultsync.ID{3}, SigAlg: vaultsync.SigEd25519}
	msg, _ := e.SigningBytes()
	sig, err := synccrypto.Sign(seed, msg)
	if err != nil {
		t.Fatal(err)
	}
	e.Sig = sig
	if err := e.Verify(pub); err != nil {
		t.Fatalf("an envelope signed with a generated device key fails verification: %v", err)
	}
}

func TestSigningKeysAreFresh(t *testing.T) {
	a, _, _ := synccrypto.GenerateSigningKey()
	b, _, _ := synccrypto.GenerateSigningKey()
	if bytes.Equal(a, b) {
		t.Fatal("two generated signing keys are equal")
	}
}
