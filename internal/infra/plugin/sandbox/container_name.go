package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
)

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
	// A leading letter, because a name is also a registry key and a directory under Packages, and a
	// purely numeric first character is the kind of thing some Windows API objects to at the worst
	// possible moment. It costs one character of a name that has room to spare.
	return "x" + hex.EncodeToString(sum[:])[:containerNameHexDigits]
}
