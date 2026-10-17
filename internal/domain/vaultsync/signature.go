package vaultsync

import (
	"crypto/ed25519"
	"fmt"
)

// SigAlg identifies a signature algorithm. Every signed structure carries one so a hybrid
// Ed25519 + ML-DSA scheme can be added later without a protocol major.
type SigAlg uint16

// SigEd25519 is the only algorithm suite 1 accepts.
const SigEd25519 SigAlg = 1

// VerifySignature checks a signature and fails closed on any unknown algorithm or malformed size.
func VerifySignature(alg SigAlg, publicKey, message, signature []byte) error {
	if alg != SigEd25519 {
		return fmt.Errorf("algorithm %d: %w", alg, ErrUnsupportedSigAlg)
	}
	if len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), message, signature) {
		return ErrBadSignature
	}
	return nil
}
