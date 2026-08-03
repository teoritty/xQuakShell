package usecase

import (
	"crypto/rand"
	"encoding/hex"
)

// newRandomID returns a random 32-character hex string (128 bits of entropy).
// It backs both session ids and operation ids: both are opaque keys that must
// be unique and are never parsed.
func newRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
