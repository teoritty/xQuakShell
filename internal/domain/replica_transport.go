package domain

import (
	"context"
	"errors"
)

// ErrReplicaConflict indicates the remote moved since it was last read, so the push was refused.
//
// It is not a failure to report to the user: it means another device wrote first, and the answer is
// to read again and merge rather than to overwrite. Overwriting is the one thing compare-and-swap
// exists to prevent.
var ErrReplicaConflict = errors.New("replica changed since it was read")

// ErrReplicaEmpty indicates an attempt to push a replica with nothing in it.
//
// Pushing nothing is how a wiped or hostile server would ask a healthy device to erase itself on the
// next fetch, so it is refused at the transport rather than treated as an unusually small document.
// Deletion never travels over the wire (I9): what a remote no longer has is proposed to the user,
// never applied.
var ErrReplicaEmpty = errors.New("refusing to push an empty replica")

// ReplicaTransport is implemented by a plugin, and moves opaque bytes to and from wherever the
// user's other devices can reach (ADR-022, port A).
//
// Two properties are the entire content of this port, and everything else about a transport is its
// author's business.
//
// The bytes are opaque. They are sealed before they get here and opened after they come back, so a
// transport - and the server behind it - carries something it cannot read. That is why this port
// takes no scope contents, no connection and no secret: there is nothing here to leak.
//
// The token is for compare-and-swap and nothing else. It says "this is the version I read", so a
// push that would overwrite someone else's write is refused instead. It is emphatically NOT evidence
// of freshness: a hostile server can hand out any token it likes. Which of two payloads is newer is
// decided by the core, from a version vector sealed inside the ciphertext where the server cannot
// reach it.
type ReplicaTransport interface {
	// Fetch returns whatever the remote holds, and the token that names that version. A remote with
	// nothing stored yet returns empty bytes and an empty token, which is not an error: it is what
	// the first device to synchronise sees.
	Fetch(ctx context.Context, pluginID string) (sealed []byte, token string, err error)

	// Push stores the bytes if the remote still holds the version named by expected, and returns the
	// token of what it now holds. An empty expected means "only if there is nothing there yet".
	// A remote that has moved on returns ErrReplicaConflict.
	Push(ctx context.Context, pluginID string, sealed []byte, expected string) (token string, err error)
}
