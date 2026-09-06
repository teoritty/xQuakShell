package domain

import (
	"testing"
	"time"
)

// A secret born expired must drop the plaintext rather than merely refuse to lend it.
//
// Through the exported API the two are indistinguishable - Use refuses either way - so nothing in
// ephemeral_test.go can see the difference, and a change from `ttl <= 0` to `ttl < 0` survives every
// test there. What it would cost is real: the bytes would stay reachable in memory for the lifetime
// of a value that can never hand them out, on the exact path where a caller made a mistake about how
// long a credential should live.
func TestSecretBornExpiredRetainsNoBytes(t *testing.T) {
	issued := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	for _, ttl := range []time.Duration{0, -time.Second} {
		secret := NewEphemeralSecretAt(issued, []byte("hunter2"), ttl)
		if len(secret.b) != 0 {
			t.Errorf("ttl %v kept %d bytes of plaintext in a secret that can never be used", ttl, len(secret.b))
		}
	}
}
