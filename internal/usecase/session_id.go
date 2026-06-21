package usecase

import (
	"crypto/rand"
	"encoding/hex"
)

// generateSessionID returns a random 32-character hex string (128 bits of entropy).
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
