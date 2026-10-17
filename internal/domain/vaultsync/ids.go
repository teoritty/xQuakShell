package vaultsync

import (
	"encoding/hex"
	"fmt"
	"io"
)

// IDSize is the byte length of vault, record, device, account and commit identifiers.
const IDSize = 16

// ID is a random 128-bit identifier. One type serves every kind of id; what keeps a record id from
// being accepted as a vault id is position in the signed canonical encoding, not the Go type.
type ID [IDSize]byte

// NewID draws an identifier from r, which is crypto/rand in production and a fixed reader in
// tests. The all-zero id is reserved to mean "absent", so drawing it is an error rather than a
// silent collision with every unset field.
func NewID(r io.Reader) (ID, error) {
	var id ID
	if _, err := io.ReadFull(r, id[:]); err != nil {
		return ID{}, fmt.Errorf("generate sync id: %w", err)
	}
	if id.IsZero() {
		return ID{}, ErrZeroID
	}
	return id, nil
}

// ParseID reads the lower-case hex form String produces and nothing else. Accepting upper case as
// well would give one id two spellings, and the server or a map would then hold one record twice.
func ParseID(s string) (ID, error) {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != IDSize {
		return ID{}, fmt.Errorf("%w: %q", ErrMalformedID, s)
	}
	var id ID
	copy(id[:], raw)
	if id.IsZero() || id.String() != s {
		return ID{}, fmt.Errorf("%w: %q", ErrMalformedID, s)
	}
	return id, nil
}

// String renders the id as 32 lower-case hex characters.
func (id ID) String() string { return hex.EncodeToString(id[:]) }

// IsZero reports the reserved "absent" value.
func (id ID) IsZero() bool { return id == (ID{}) }
