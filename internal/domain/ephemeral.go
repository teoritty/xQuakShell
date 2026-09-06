package domain

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// EphemeralRedacted is the text every formatting path prints in place of the bytes. It is
// exported so a test can assert on it and the UI can recognise it, rather than each of them
// carrying its own copy of a string that must agree with this one.
const EphemeralRedacted = "[ephemeral secret]"

// errNilSecretCallback reports a caller that reached Use without a callback. It is unexported
// because nothing outside this package needs to tell it apart from any other refusal: it marks a
// programming mistake, and the only reason it is an error rather than a panic is that the paths
// carrying these secrets are credential paths, where crashing is the worse of the two answers.
var errNilSecretCallback = errors.New("ephemeral secret: no callback was supplied")

// EphemeralSecret carries a secret that must never reach disk, such as a lease fetched from an
// external store on behalf of a plugin (ADR-022).
//
// The bytes are unexported and there is no getter: a caller borrows them inside Use and the borrow
// ends when the callback returns. That is what keeps the set of places holding a copy small enough
// to enumerate during a review.
//
// The type refuses to serialize and redacts under every formatting verb, which closes the two
// routes a secret takes by accident - into the vault or one of its backups, and into a log line.
// Neither guard stops a caller that deliberately copies the bytes out of Use, and neither stops a
// memory dump; those remain the business of the logging rules and of the same-user boundary.
type EphemeralSecret struct {
	b       []byte
	expires time.Time
}

// NewEphemeralSecret mints a secret that expires ttl from now. Prefer NewEphemeralSecretAt where
// the issuing instant is already known, so the result does not depend on the system clock.
func NewEphemeralSecret(b []byte, ttl time.Duration) EphemeralSecret {
	return NewEphemeralSecretAt(time.Now(), b, ttl)
}

// NewEphemeralSecretAt mints a secret issued at a given instant, expiring ttl later.
//
// A non-positive ttl yields the zero value, which is expired and holds nothing. Reading a mistake
// about the lifetime of a credential as "already over" is the only safe direction; the alternative
// reading, "forever", is how a lease quietly becomes a stored password.
func NewEphemeralSecretAt(issued time.Time, b []byte, ttl time.Duration) EphemeralSecret {
	if ttl <= 0 {
		return EphemeralSecret{}
	}
	return EphemeralSecret{
		b:       append([]byte(nil), b...),
		expires: issued.Add(ttl),
	}
}

// Expired reports whether the secret's lifetime has elapsed at the given instant. The zero value is
// expired, so a struct field nobody filled in is inert rather than a usable empty secret.
func (s EphemeralSecret) Expired(now time.Time) bool {
	return !now.Before(s.expires)
}

// Use lends the bytes to fn for the duration of the call and returns whatever fn returns.
//
// An expired secret refuses and fn does not run: an external store can revoke at any moment, and
// degrading to the last known value is the failure mode a TTL exists to prevent. The callback
// receives a copy, so a callback that writes to what it was lent cannot corrupt a secret other
// holders of this value still rely on.
func (s EphemeralSecret) Use(now time.Time, fn func([]byte) error) error {
	if fn == nil {
		return errNilSecretCallback
	}
	if s.Expired(now) {
		return ErrEphemeralSecretExpired
	}
	return fn(append([]byte(nil), s.b...))
}

// MarshalJSON always fails, which is the whole point of the method existing.
//
// encoding/json wraps this in a *json.MarshalerError that unwraps to it, so the refusal survives
// being raised from a field several levels inside the value someone actually marshalled - which is
// how a secret would reach a serializer in practice.
func (s EphemeralSecret) MarshalJSON() ([]byte, error) {
	return nil, ErrEphemeralSecretNotSerializable
}

// Format redacts under every verb, including the ones a reader would not expect a secret to meet.
//
// fmt.Formatter takes precedence over Stringer and GoStringer, so this one method covers %v, %s,
// %q, %#v and everything else in one place. The interface cannot report a write failure and fmt
// records one on the State itself, so the error here is discarded deliberately rather than lost.
func (s EphemeralSecret) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, EphemeralRedacted)
}

// String returns the redaction, so a deliberate call agrees with what Format prints.
func (s EphemeralSecret) String() string {
	return EphemeralRedacted
}

// GoString returns the redaction wrapped in the type name, so %#v output stays recognisable as
// this type without disclosing what it holds.
func (s EphemeralSecret) GoString() string {
	return "domain.EphemeralSecret{" + EphemeralRedacted + "}"
}
