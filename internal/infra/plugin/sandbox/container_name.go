package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContainerNamePrefix marks a profile as this application's. The orphan sweep deletes profiles by
// name, so it needs a way to be certain it is only ever deleting ours: without a prefix the sweep
// would be matching a bare hex string and one day it would match somebody else's.
const ContainerNamePrefix = "xqs"

// containerNameHexDigits is how much of the digest the name carries. An AppContainer profile name is
// length- and charset-limited, and 128 bits is far past the point where a birthday collision is a
// thing that happens: an installation would need on the order of 2^64 distinct plugin instances
// before two shared a name.
const containerNameHexDigits = 32

// ContainerName is the AppContainer profile name for one plugin instance, derived by hashing its
// identity rather than by sanitising it.
//
// Sanitising would be the obvious implementation and it is the dangerous one. Profile names admit a
// restricted charset and a bounded length, so any sanitising function maps some pair of distinct
// identities onto one name — and two plugins sharing a container name share a container SID, which
// means they share every ACE granted to it. That is cross-plugin file access, handed out by the
// mechanism whose entire purpose is to prevent it. Plugin ids come from plugin authors, so the
// input is attacker-chosen and the collision is not hypothetical: an author who wants to read
// another plugin's files only has to pick an id that sanitises to the same string.
//
// A hash cannot be steered that way. The caller is responsible for the identity being unambiguous —
// see PluginInstanceKey, which length-prefixes its parts for exactly that reason.
func ContainerName(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	// The prefix also gives the name a leading letter, which a registry key and a directory under
	// Packages both prefer over a digit.
	return ContainerNamePrefix + hex.EncodeToString(sum[:])[:containerNameHexDigits]
}
