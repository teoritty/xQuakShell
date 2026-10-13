package synccrypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
)

// GenerateSigningKey creates an Ed25519 key for a device (DSK) or an account identity (AIK). The
// 32-byte seed is the storage form; verification lives in vaultsync, which both sides share.
func GenerateSigningKey() (seed, public []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("sync: generate signing key: %w", err)
	}
	defer clear(priv)
	return priv.Seed(), pub, nil
}

// Sign signs message with the key a seed expands to, and wipes the expanded key afterwards so the
// only copy left is the seed the caller already holds.
func Sign(seed, message []byte) ([]byte, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("sync: signing seed is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	defer clear(priv)
	return ed25519.Sign(priv, message), nil
}
