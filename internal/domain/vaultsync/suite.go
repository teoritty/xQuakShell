package vaultsync

import "fmt"

// SuiteID names a fixed set of cryptographic choices. It is a number, never a negotiation: a client
// compiles in what each suite means and refuses anything below its minimum, so a server cannot talk
// it into weaker parameters by describing them. Raising any parameter is a new suite id.
type SuiteID uint16

const (
	// Suite1 is OPAQUE ristretto255 with Argon2id, HPKE ML-KEM-768+X25519 / HKDF-SHA256 /
	// ChaCha20-Poly1305 for key wrapping, XChaCha20-Poly1305 for data, Ed25519 for signatures.
	Suite1 SuiteID = 1
	// CurrentSuite is what this build writes.
	CurrentSuite = Suite1
	// MinAcceptedSuite is the oldest suite this build reads.
	MinAcceptedSuite = Suite1
)

// Argon2Params are a suite's password-stretching costs.
type Argon2Params struct {
	MemoryKiB  uint32
	Iterations uint32
	Threads    uint8
	KeyLen     uint32
}

// SuiteParams carries the parts of a suite the domain needs for size validation. The cipher
// implementations live in infrastructure; these numbers let the domain refuse a malformed
// envelope before any of them run.
type SuiteParams struct {
	ID     SuiteID
	Argon2 Argon2Params
	// WrappedDEKSize: 24-byte XChaCha nonce, 32-byte key, 16-byte tag.
	WrappedDEKSize int
	// AEADOverhead: what XChaCha20-Poly1305 adds to a sealed body, nonce plus tag.
	AEADOverhead int
	// SigningKeySize: an Ed25519 public key, for AIK and DSK.
	SigningKeySize int
	// EncryptionKeySize: an ML-KEM-768 encapsulation key (1184) plus an X25519 key (32), the AEK.
	EncryptionKeySize int
	// VKWrapSize: HPKE encapsulation (1088 + 32), a 32-byte vault key, a 16-byte tag.
	VKWrapSize int
}

var suites = map[SuiteID]SuiteParams{
	Suite1: {
		ID:                Suite1,
		Argon2:            Argon2Params{MemoryKiB: 256 * 1024, Iterations: 3, Threads: 4, KeyLen: 32},
		WrappedDEKSize:    72,
		AEADOverhead:      40,
		SigningKeySize:    32,
		EncryptionKeySize: 1216,
		VKWrapSize:        1168,
	},
}

// LookupSuite returns a suite's parameters, refusing one below minimum before refusing one it
// does not know, so a downgrade attempt is reported as what it is.
func LookupSuite(id, minimum SuiteID) (SuiteParams, error) {
	if id < minimum {
		return SuiteParams{}, fmt.Errorf("suite %d, minimum %d: %w", id, minimum, ErrSuiteBelowMinimum)
	}
	p, ok := suites[id]
	if !ok {
		return SuiteParams{}, fmt.Errorf("suite %d: %w", id, ErrUnsupportedSuite)
	}
	return p, nil
}
