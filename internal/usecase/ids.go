package usecase

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync/atomic"
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

// surfaceIDPrefix keeps plugin surface ids in an id space disjoint from session ids by
// construction (ADR-015 §1). A surface is not a session and must never be mistaken for one by a
// lookup handed the wrong id: with the prefix, such a mistake is a miss rather than a match.
const surfaceIDPrefix = "srf-"

// surfaceIDSeq makes surface ids unique even if the entropy source fails.
//
// randomHex degrades to a constant on failure, which is harmless for the vault-local labels it was
// written for and is not harmless here: two surfaces sharing an id would have one of them refused
// by SurfaceRegistry.Add for the rest of the process's life. The counter guarantees uniqueness on
// its own; the random half is there so an id is not guessable from watching another one.
var surfaceIDSeq atomic.Uint64

// newSurfaceID mints an identifier for a plugin surface.
func newSurfaceID() string {
	return surfaceIDPrefix + strconv.FormatUint(surfaceIDSeq.Add(1), 36) + "-" + randomHex(8)
}
