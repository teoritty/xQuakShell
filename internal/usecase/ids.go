package usecase

import (
	"crypto/rand"
	"encoding/hex"
)

// randomHex returns a hex string of n random bytes (2n characters).
//
// It is used for short, collision-resistant local identifiers (connection user
// IDs, jump hop IDs) generated while importing external configuration. On a
// failing entropy source it degrades to a fixed value rather than panicking:
// these IDs are vault-local labels, not security tokens, and the callers'
// uniqueness checks (Connection.Validate) surface a collision as a validation
// error instead of a crash.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}
