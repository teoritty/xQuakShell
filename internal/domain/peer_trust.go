package domain

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
)

// MaxPeerTrustMaterial bounds the size of trusted material.
//
// The material arrives from a plugin. Without a ceiling it would inflate the encrypted vault,
// which is read and rewritten whole on every change.
const MaxPeerTrustMaterial = 64 << 10

var (
	// ErrPeerUnknown means the subject is being seen for the first time.
	ErrPeerUnknown = errors.New("peer trust: subject not known")
	// ErrPeerMismatch means the subject is known but the material differs.
	ErrPeerMismatch = errors.New("peer trust: material differs from the trusted one")
	// ErrPeerMaterialTooLarge means the material is over the ceiling.
	ErrPeerMaterialTooLarge = errors.New("peer trust: material exceeds the size limit")
	// ErrPeerMaterialEmpty means no material was presented.
	ErrPeerMaterialEmpty = errors.New("peer trust: material is empty")

	// ErrPeerTrustPromptBusy means the session already waits on a different question.
	//
	// Refusing the second question is what keeps consent honest. A prompt that could be
	// overwritten would let a plugin raise a harmless fingerprint, wait for the dialog to
	// appear, and swap the material underneath it - the user would then click "trust" on one
	// value and store another.
	ErrPeerTrustPromptBusy = errors.New("peer trust: another decision is already pending for this session")

	// ErrPeerTrustPromptStale means the answer names a fingerprint the session is not asking about.
	//
	// The caller echoes back the fingerprint it displayed, so an answer that raced a replaced
	// prompt is refused instead of being applied to whatever is pending now.
	ErrPeerTrustPromptStale = errors.New("peer trust: the decision does not match the pending question")

	// ErrPeerTrustNoPending means the session was never asked anything.
	//
	// Answering without a question would be a way to write trust outside a check the core
	// itself started - the defect ResolveHostKey was cured of.
	ErrPeerTrustNoPending = errors.New("peer trust: no pending decision for this session")

	// ErrPeerTrustNotWaiting means the session cannot take a trust question in its current state.
	//
	// Only a connecting session can be stopped by one. Without this a plugin could drag its own
	// established session back into trust-required at any moment.
	ErrPeerTrustNotWaiting = errors.New("peer trust: session is not connecting")
)

// PeerTrustEntry is a remote identity the user has confirmed.
//
// The type knows no protocol term at all: not "host", not "certificate", not "key". It knows a
// scope, a subject and opaque bytes. That property is the whole reason this mechanism is separate
// from known_hosts, which does the opposite - it stores OpenSSH-format lines and parses them as
// SSH keys.
type PeerTrustEntry struct {
	// Scope is who the trust belongs to: the plugin id, taken from the session binding rather
	// than from the parameters of the call.
	Scope string `json:"scope"`
	// Subject is what the trust is bound to. The core derives it from the connection record.
	Subject string `json:"subject"`
	// Material is the bytes observed on the other side.
	//
	// The original is stored, not a fingerprint. Comparing originals does not rest on a hash
	// being collision resistant, and the core computes the fingerprint itself and only to show a
	// human - so what was shown and what was stored cannot drift apart.
	Material []byte    `json:"material"`
	AddedAt  time.Time `json:"addedAt"`
}

// PeerTrustRepository stores confirmed trust.
//
// The port deliberately knows nothing about sessions or about the user: it can find, put, list and
// remove. Who may do so is decided by the use case, and decided in one place.
type PeerTrustRepository interface {
	// Find returns the entry, or nil when there is none.
	//
	// Absence is not an error: it is an ordinary first connection, and turning it into one would
	// force every caller to tell "not known" apart from "storage broke".
	Find(scope, subject string) (*PeerTrustEntry, error)
	// Put creates or REPLACES the entry for the (scope, subject) pair.
	//
	// Replaces, precisely: two entries for one pair would mean either material counts as
	// trusted, and a key rotation would stop meaning anything.
	Put(ctx context.Context, entry PeerTrustEntry) error
	List() ([]PeerTrustEntry, error)
	Remove(ctx context.Context, scope, subject string) error
}

// PeerFingerprint renders material as a fingerprint for a human to read.
//
// The algorithm is named inside the value. Bare base64 is uninterpretable a year later - nothing
// says what it is a hash of - while the prefix leaves a path to changing the algorithm without
// silently invalidating values already shown to the user.
func PeerFingerprint(material []byte) string {
	sum := sha256.Sum256(material)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}
