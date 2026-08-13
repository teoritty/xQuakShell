package domain

import "errors"

const (
	// MaxRemoteWalkDepth bounds how far a recursive walk of a remote tree will descend.
	//
	// The structure being walked is the server's to invent. A directory tree that is infinitely
	// deep - /a/a/a/a/... - costs a hostile or compromised server nothing to serve and is not
	// exotic: any custom SFTP implementation or FUSE mount can generate one on demand. Both walks
	// recursed on it without a bound, so the client either exhausted its stack and crashed or grew
	// its result slice until it ran out of memory. Cancellation was the only brake, and it needs a
	// user who has noticed.
	//
	// 64 is far past anything a real tree reaches - a deeply nested dependency directory is around
	// thirty - and far short of a depth that costs anything to refuse.
	MaxRemoteWalkDepth = 64

	// MaxRemoteWalkEntries bounds how many nodes one walk may enumerate.
	//
	// The depth cap alone stops the infinite tree, which is the attack. This is the backstop for
	// the shape it does not cover: broad rather than deep, a server answering every ReadDir with
	// another thousand names. A million entries is more than any transfer a person starts on
	// purpose and still bounded.
	MaxRemoteWalkEntries = 1_000_000
)

// ErrRemoteWalkTooDeep indicates a remote directory tree exceeded MaxRemoteWalkDepth.
//
// It is reported rather than silently truncated: a partial answer presented as a whole one is how a
// recursive chmod appears to have finished while leaving most of the tree untouched.
var ErrRemoteWalkTooDeep = errors.New("remote directory tree is deeper than this client will walk")

// ErrRemoteWalkTooLarge indicates a remote directory tree exceeded MaxRemoteWalkEntries.
var ErrRemoteWalkTooLarge = errors.New("remote directory tree has more entries than this client will walk")
