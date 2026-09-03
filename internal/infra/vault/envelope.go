package vault

import (
	"bytes"
	"encoding/json"
	"fmt"

	"xquakshell/internal/domain"
)

// CurrentEnvelopeVersion is the on-disk envelope format this build writes.
//
// It is a version of its own, separate from domain.CurrentVaultVersion, because the two describe
// different things. The vault version describes the JSON inside the ciphertext and can only be read
// with a credential; the envelope describes how the ciphertext is wrapped and is readable with
// none. A build that cannot open a file needs to say which of the two is wrong, and only a number
// outside the encryption can tell it.
const CurrentEnvelopeVersion = 1

// legacyAgeHeader is the first line of any age v1 file. Vaults written before recovery keys existed
// are raw age files, so their first bytes are this and nothing else needs to be stored to recognise
// them - which matters, because the old format has no room to store anything.
const legacyAgeHeader = "age-encryption.org/v1"

// wrapKind names which credential opens a wrap.
type wrapKind string

const (
	wrapPassword wrapKind = "password"
	wrapRecovery wrapKind = "recovery"
)

// envelopeWrap is one credential's copy of the vault key.
type envelopeWrap struct {
	Kind wrapKind `json:"kind"`
	Data []byte   `json:"data"`
}

// envelope is the vault file: the encrypted data, plus one small blob per credential that can
// decrypt it.
//
// The indirection is the whole design. The payload is encrypted to a vault key that never changes,
// and each credential holds a separately wrapped copy of that key. Changing a password or rotating
// a recovery key rewrites a few hundred bytes and never touches the data - and, more importantly,
// unlocking with one credential does not need the other to still be there afterwards. Encrypting
// the payload to two age recipients directly would have that exact bug: every flush re-encrypts,
// and after a password unlock the recovery credential is not in memory to re-add.
type envelope struct {
	Envelope int            `json:"envelope"`
	Wraps    []envelopeWrap `json:"wraps"`
	Payload  []byte         `json:"payload"`
}

// IsLegacyAgeFile reports whether these bytes are a pre-envelope vault, written directly under the
// master password.
func IsLegacyAgeFile(raw []byte) bool {
	return bytes.HasPrefix(raw, []byte(legacyAgeHeader))
}

// parseEnvelope decodes a vault file and refuses one from a newer build.
//
// The version gate runs before anything else looks at the contents. A file whose wraps this build
// does not understand must not be reported as a bad password, or the user spends the evening
// retyping a credential that was right all along.
func parseEnvelope(raw []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("vault envelope parse: %w", err)
	}
	if env.Envelope > CurrentEnvelopeVersion {
		return nil, fmt.Errorf("vault envelope %d, this build reads %d: %w",
			env.Envelope, CurrentEnvelopeVersion, domain.ErrVaultEnvelopeTooNew)
	}
	if len(env.Payload) == 0 {
		return nil, fmt.Errorf("vault envelope has no payload: %w", domain.ErrVaultDecryptFailed)
	}
	return &env, nil
}

// marshalEnvelope encodes a vault file, refusing to write a version this build cannot read back.
//
// The check is not redundant with the one in parseEnvelope. Both directions are separate errors so
// that a newer file is never partially read and never overwritten - overwriting is the one of the
// two that destroys data.
func marshalEnvelope(env *envelope) ([]byte, error) {
	if env.Envelope > CurrentEnvelopeVersion {
		return nil, fmt.Errorf("refusing to write vault envelope %d from a build that reads %d: %w",
			env.Envelope, CurrentEnvelopeVersion, domain.ErrVaultEnvelopeTooNew)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("vault envelope marshal: %w", err)
	}
	return raw, nil
}

// wrapFor returns the wrap a given credential opens.
func (e *envelope) wrapFor(kind wrapKind) ([]byte, bool) {
	for _, w := range e.Wraps {
		if w.Kind == kind {
			return w.Data, true
		}
	}
	return nil, false
}

// setWrap replaces the wrap of this kind, or appends it when there is none.
//
// Replacing rather than appending is what revokes the old credential: a vault holds at most one
// wrap per kind, so the previous password or recovery key stops opening the file the moment this
// returns and the result is written.
func (e *envelope) setWrap(kind wrapKind, data []byte) {
	for i := range e.Wraps {
		if e.Wraps[i].Kind == kind {
			e.Wraps[i].Data = data
			return
		}
	}
	e.Wraps = append(e.Wraps, envelopeWrap{Kind: kind, Data: data})
}
