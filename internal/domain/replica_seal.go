package domain

import "errors"

// ErrReplicaKeyRequired indicates a seal or open attempted with no replication key.
var ErrReplicaKeyRequired = errors.New("replication key required")

// ErrReplicaKeyExists indicates an attempt to mint a second key over one this plugin already has.
//
// The existing key is what opens every replica that plugin has ever sealed, so replacing it
// silently would leave the user with a device that can no longer read its own history and nothing
// to explain why. Joining an existing set of devices is a separate, explicit action.
var ErrReplicaKeyExists = errors.New("replication key already exists")

// ErrReplicaKeyMalformed indicates a typed key that is not one this application ever issued.
//
// Refused at entry rather than stored, because a stored typo fails every later fetch with "could
// not be opened" - which reads as data loss rather than as a mistyped character.
var ErrReplicaKeyMalformed = errors.New("replication key is malformed")

// ErrReplicaOpenFailed indicates a sealed replica that this key does not open, or that is not a
// sealed replica at all.
//
// The two are one error on purpose, for the same reason a wrong master password and a wrong recovery
// key are one error (ADR-021): telling them apart hands a guesser the one bit that says whether the
// key is close.
var ErrReplicaOpenFailed = errors.New("sealed replica could not be opened")

// ReplicaSealer turns a replica document into the opaque bytes a transport carries, and back.
//
// It exists so that T1 is a property of the code rather than of a plugin's good behaviour: the
// document is sealed before any plugin is handed anything, so what a plugin moves is ciphertext and
// what a hostile server can read is nothing - including the hostnames and usernames, which are
// intelligence rather than "just config".
//
// The key is not the vault key. Vaults are created independently on each machine and their keys do
// not match, so replication needs a secret of its own that the user carries between devices.
type ReplicaSealer interface {
	Seal(doc ReplicaDocument, key string) ([]byte, error)
	Open(sealed []byte, key string) (ReplicaDocument, error)
}
