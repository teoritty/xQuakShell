package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// A secret that reached a serializer took a wrong turn. Returning an error turns that wrong turn
// into a test failure here instead of a line in someone's vault backup, which is the whole reason
// this type exists rather than a plain []byte.
func TestEphemeralSecretRefusesToSerialize(t *testing.T) {
	secret := domain.NewEphemeralSecret([]byte("hunter2"), time.Minute)

	_, err := json.Marshal(secret)
	if err == nil {
		t.Fatal("json.Marshal accepted an ephemeral secret")
	}
	if !errors.Is(err, domain.ErrEphemeralSecretNotSerializable) {
		t.Fatalf("err = %v, want ErrEphemeralSecretNotSerializable", err)
	}
}

// The realistic accident is not marshalling the secret itself but marshalling a struct that
// happens to carry one. If the refusal did not propagate out of an enclosing value, the guard
// would only catch the mistake nobody makes.
func TestEphemeralSecretRefusesToSerializeInsideAStruct(t *testing.T) {
	holder := struct {
		Name   string                 `json:"name"`
		Secret domain.EphemeralSecret `json:"secret"`
	}{
		Name:   "corp-vault",
		Secret: domain.NewEphemeralSecret([]byte("hunter2"), time.Minute),
	}

	out, err := json.Marshal(holder)
	if err == nil {
		t.Fatalf("json.Marshal accepted a struct holding an ephemeral secret: %s", out)
	}
	if !errors.Is(err, domain.ErrEphemeralSecretNotSerializable) {
		t.Fatalf("err = %v, want ErrEphemeralSecretNotSerializable", err)
	}
}

// The way a secret actually reaches a log is a %v on the struct that holds it, so every verb has
// to redact - not just the one the author of this type happened to think of.
func TestEphemeralSecretRedactsEveryFormattingVerb(t *testing.T) {
	const plaintext = "hunter2"
	secret := domain.NewEphemeralSecret([]byte(plaintext), time.Minute)

	for _, format := range []string{"%v", "%s", "%q", "%d", "%x", "%#v", "%+v"} {
		got := fmt.Sprintf(format, secret)
		if strings.Contains(got, plaintext) {
			t.Errorf("%s leaked the secret: %s", format, got)
		}
		if !strings.Contains(got, domain.EphemeralRedacted) {
			t.Errorf("%s = %q, want it to contain %q", format, got, domain.EphemeralRedacted)
		}
	}
}

// A logger is handed the enclosing struct, never the field. Redaction that only works when the
// secret is formatted on its own protects nothing.
func TestEphemeralSecretRedactsInsideAStruct(t *testing.T) {
	const plaintext = "hunter2"
	holder := struct {
		Name   string
		Secret domain.EphemeralSecret
	}{Name: "corp-vault", Secret: domain.NewEphemeralSecret([]byte(plaintext), time.Minute)}

	for _, format := range []string{"%v", "%+v", "%#v"} {
		if got := fmt.Sprintf(format, holder); strings.Contains(got, plaintext) {
			t.Errorf("%s leaked the secret through the enclosing struct: %s", format, got)
		}
	}
}

// String is what a caller reaches for deliberately, and it must agree with Format. Two redaction
// paths that could disagree are two chances for one of them to be forgotten.
func TestEphemeralSecretStringIsRedacted(t *testing.T) {
	secret := domain.NewEphemeralSecret([]byte("hunter2"), time.Minute)

	if got := secret.String(); got != domain.EphemeralRedacted {
		t.Fatalf("String() = %q, want %q", got, domain.EphemeralRedacted)
	}
	if got := secret.GoString(); !strings.Contains(got, domain.EphemeralRedacted) {
		t.Fatalf("GoString() = %q, want it to contain %q", got, domain.EphemeralRedacted)
	}
}

// Use is the only way to reach the bytes, and it must hand over what was put in - otherwise the
// type is safe and useless.
func TestEphemeralSecretLendsItsBytesInsideUse(t *testing.T) {
	const plaintext = "hunter2"
	secret := domain.NewEphemeralSecret([]byte(plaintext), time.Minute)

	var seen string
	err := secret.Use(time.Now(), func(b []byte) error {
		seen = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("Use returned %v", err)
	}
	if seen != plaintext {
		t.Fatalf("Use lent %q, want %q", seen, plaintext)
	}
}

// The callback's error is the caller's error. Swallowing it would make a failed use look like a
// successful one at the exact moment a credential was involved.
func TestEphemeralSecretUsePropagatesCallbackError(t *testing.T) {
	secret := domain.NewEphemeralSecret([]byte("hunter2"), time.Minute)
	sentinel := errors.New("dial failed")

	err := secret.Use(time.Now(), func([]byte) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Use returned %v, want the callback's error", err)
	}
}

// The caller keeps writing to the buffer it handed over, or reuses it. Sharing the array would
// mean the stored secret changes underneath, and that a caller can read it back out later by
// holding on to the slice it passed in.
func TestEphemeralSecretCopiesTheBytesItIsGiven(t *testing.T) {
	buf := []byte("hunter2")
	secret := domain.NewEphemeralSecret(buf, time.Minute)

	buf[0] = 'X'

	var seen string
	if err := secret.Use(time.Now(), func(b []byte) error {
		seen = string(b)
		return nil
	}); err != nil {
		t.Fatalf("Use returned %v", err)
	}
	if seen != "hunter2" {
		t.Fatalf("the secret shares its array with the caller's buffer: %q", seen)
	}
}

// The borrowed slice must not be the stored one either. A callback that writes to what it was
// lent would corrupt a secret every other caller still holds.
func TestEphemeralSecretLendsACopyNotTheOriginal(t *testing.T) {
	secret := domain.NewEphemeralSecret([]byte("hunter2"), time.Minute)

	if err := secret.Use(time.Now(), func(b []byte) error {
		b[0] = 'X'
		return nil
	}); err != nil {
		t.Fatalf("Use returned %v", err)
	}

	var seen string
	if err := secret.Use(time.Now(), func(b []byte) error {
		seen = string(b)
		return nil
	}); err != nil {
		t.Fatalf("Use returned %v", err)
	}
	if seen != "hunter2" {
		t.Fatalf("a callback mutated the stored secret: %q", seen)
	}
}

// An expired lease refuses rather than degrades (V10). A secret that keeps working past its TTL
// is exactly the revocation blindness the TTL was added to bound.
func TestEphemeralSecretRefusesUseAfterItExpires(t *testing.T) {
	issued := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	secret := domain.NewEphemeralSecretAt(issued, []byte("hunter2"), time.Minute)

	if secret.Expired(issued.Add(59 * time.Second)) {
		t.Fatal("the secret expired before its TTL elapsed")
	}
	if !secret.Expired(issued.Add(time.Minute)) {
		t.Fatal("the secret is still valid at the instant its TTL elapsed")
	}

	called := false
	err := secret.Use(issued.Add(time.Minute), func([]byte) error {
		called = true
		return nil
	})
	if !errors.Is(err, domain.ErrEphemeralSecretExpired) {
		t.Fatalf("err = %v, want ErrEphemeralSecretExpired", err)
	}
	if called {
		t.Fatal("the callback ran with an expired secret")
	}
}

// A non-positive TTL is a caller mistake, and the safe reading of a mistake about the lifetime of
// a credential is "already over", never "forever".
func TestEphemeralSecretWithNonPositiveTTLIsBornExpired(t *testing.T) {
	issued := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	for _, ttl := range []time.Duration{0, -time.Second} {
		secret := domain.NewEphemeralSecretAt(issued, []byte("hunter2"), ttl)
		if !secret.Expired(issued) {
			t.Errorf("ttl %v produced a secret that is not expired at issue time", ttl)
		}
		if err := secret.Use(issued, func([]byte) error { return nil }); !errors.Is(err, domain.ErrEphemeralSecretExpired) {
			t.Errorf("ttl %v: Use returned %v, want ErrEphemeralSecretExpired", ttl, err)
		}
	}
}

// The zero value is what a struct field holds when nothing filled it in. It must be inert rather
// than a usable empty secret, and it must not panic - a nil dereference here would be a crash on
// a credential path.
func TestZeroEphemeralSecretIsInertAndSafe(t *testing.T) {
	var secret domain.EphemeralSecret

	if !secret.Expired(time.Now()) {
		t.Fatal("the zero value is not expired")
	}
	if err := secret.Use(time.Now(), func([]byte) error { return nil }); !errors.Is(err, domain.ErrEphemeralSecretExpired) {
		t.Fatalf("Use on the zero value returned %v, want ErrEphemeralSecretExpired", err)
	}
	if got := secret.String(); got != domain.EphemeralRedacted {
		t.Fatalf("String() on the zero value = %q, want %q", got, domain.EphemeralRedacted)
	}
	if _, err := json.Marshal(secret); !errors.Is(err, domain.ErrEphemeralSecretNotSerializable) {
		t.Fatalf("json.Marshal of the zero value returned %v, want the refusal", err)
	}
}

// A nil callback is a programming error, not a reason to panic on a path that is handling a
// credential.
func TestEphemeralSecretUseRejectsANilCallback(t *testing.T) {
	secret := domain.NewEphemeralSecret([]byte("hunter2"), time.Minute)

	if err := secret.Use(time.Now(), nil); err == nil {
		t.Fatal("Use accepted a nil callback")
	}
}
