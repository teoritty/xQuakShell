package vaultsync_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func testKey(seed byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
}

func signedEnvelope(t *testing.T, key ed25519.PrivateKey) *vaultsync.Envelope {
	t.Helper()
	e := &vaultsync.Envelope{
		Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 1,
		Suite: vaultsync.Suite1, WrappedDEK: bytes.Repeat([]byte{7}, 72),
		Body: bytes.Repeat([]byte{9}, vaultsync.MinPaddedSize+40), Author: vaultsync.ID{4},
		SigAlg: vaultsync.SigEd25519,
	}
	msg, err := e.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	e.Sig = ed25519.Sign(key, msg)
	return e
}

func cloneEnvelope(e *vaultsync.Envelope) *vaultsync.Envelope {
	c := *e
	c.WrappedDEK = bytes.Clone(e.WrappedDEK)
	c.Body = bytes.Clone(e.Body)
	c.Sig = bytes.Clone(e.Sig)
	return &c
}

func TestEnvelopeVerifies(t *testing.T) {
	key := testKey(1)
	e := signedEnvelope(t, key)
	if err := e.Verify(key.Public().(ed25519.PublicKey)); err != nil {
		t.Fatalf("a correctly signed envelope failed: %v", err)
	}
}

func TestEnvelopeRejectsOtherKey(t *testing.T) {
	e := signedEnvelope(t, testKey(1))
	err := e.Verify(testKey(2).Public().(ed25519.PublicKey))
	if !errors.Is(err, vaultsync.ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
}

// Every single-byte change anywhere in a signed envelope must fail verification with an integrity
// error. This is the exhaustive tamper test of spec section 12.2 at the domain level.
func TestEnvelopeAnyTamperFails(t *testing.T) {
	key := testKey(1)
	pub := key.Public().(ed25519.PublicKey)
	base := signedEnvelope(t, key)
	var mutators []func(*vaultsync.Envelope)
	mutators = append(mutators,
		func(e *vaultsync.Envelope) { e.Vault[0] ^= 1 },
		func(e *vaultsync.Envelope) { e.Record[15] ^= 1 },
		func(e *vaultsync.Envelope) { e.Version++ },
		func(e *vaultsync.Envelope) { e.Epoch++ },
		func(e *vaultsync.Envelope) { e.Suite++ },
		func(e *vaultsync.Envelope) { e.Deleted = !e.Deleted },
		// A live version relabelled as an empty tombstone: structurally valid, so only the
		// signature can catch a server deleting a record on its own.
		func(e *vaultsync.Envelope) { e.Deleted, e.WrappedDEK, e.Body = true, nil, nil },
		func(e *vaultsync.Envelope) { e.Author[3] ^= 1 },
		func(e *vaultsync.Envelope) { e.SigAlg++ },
	)
	for i := range base.WrappedDEK {
		mutators = append(mutators, func(e *vaultsync.Envelope) { e.WrappedDEK[i] ^= 0x80 })
	}
	for i := range base.Body {
		mutators = append(mutators, func(e *vaultsync.Envelope) { e.Body[i] ^= 0x01 })
	}
	for i := range base.Sig {
		mutators = append(mutators, func(e *vaultsync.Envelope) { e.Sig[i] ^= 0x40 })
	}
	for i, mutate := range mutators {
		e := cloneEnvelope(base)
		mutate(e)
		if err := e.Verify(pub); !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Fatalf("mutation %d verified or failed without an integrity error (%v)", i, err)
		}
	}
}

func TestEnvelopeValidate(t *testing.T) {
	good := func() *vaultsync.Envelope { return signedEnvelope(t, testKey(1)) }
	cases := map[string]func(*vaultsync.Envelope){
		"zero vault":          func(e *vaultsync.Envelope) { e.Vault = vaultsync.ID{} },
		"zero record":         func(e *vaultsync.Envelope) { e.Record = vaultsync.ID{} },
		"zero author":         func(e *vaultsync.Envelope) { e.Author = vaultsync.ID{} },
		"version zero":        func(e *vaultsync.Envelope) { e.Version = 0 },
		"short wrapped key":   func(e *vaultsync.Envelope) { e.WrappedDEK = e.WrappedDEK[:71] },
		"body below floor":    func(e *vaultsync.Envelope) { e.Body = e.Body[:vaultsync.MinPaddedSize+39] },
		"tombstone with body": func(e *vaultsync.Envelope) { e.Deleted = true },
		"short signature":     func(e *vaultsync.Envelope) { e.Sig = e.Sig[:63] },
		"unknown suite":       func(e *vaultsync.Envelope) { e.Suite = 99 },
	}
	for name, mutate := range cases {
		e := good()
		mutate(e)
		if err := e.Validate(); !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Errorf("%s: Validate err = %v, want an integrity error", name, err)
		}
	}
	tomb := good()
	tomb.Deleted, tomb.WrappedDEK, tomb.Body = true, nil, nil
	if err := tomb.Validate(); err != nil {
		t.Errorf("an empty tombstone failed validation: %v", err)
	}
}

func TestEnvelopeDigestCoversSignature(t *testing.T) {
	e := signedEnvelope(t, testKey(1))
	d1, _ := e.Digest()
	e.Sig[0] ^= 1
	d2, _ := e.Digest()
	if d1 == d2 {
		t.Fatal("digest ignores the signature; the Merkle tree would not commit to who signed")
	}
}
