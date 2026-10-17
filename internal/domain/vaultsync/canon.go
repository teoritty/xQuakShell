package vaultsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// Canon builds xqs-canon v1, the encoding every signature and hash in the sync protocol covers.
//
// Every field, integers included, is a big-endian uint32 length followed by its bytes, in an order
// the caller fixes. The length prefixes are what make the encoding injective: without them
// ("ab","c") and ("a","bc") would encode alike and a signature over one would verify for the other.
// JSON is not used here because two JSON encoders disagree about the same value often enough to
// have a CVE history; length-prefixed bytes leave nothing to disagree about. JSON stays the wire
// format, and verifiers rebuild these bytes from the parsed fields.
//
// Errors are sticky rather than returned per call, so a chain of appends reads as the field list
// it encodes; Encoded and Sum report the first one.
type Canon struct {
	buf []byte
	err error
}

// NewCanon starts an encoding whose first field is a domain-separation label, so bytes signed for
// one purpose can never verify for another.
func NewCanon(label string) *Canon {
	c := &Canon{}
	return c.String(label)
}

// Bytes appends one length-prefixed field.
func (c *Canon) Bytes(b []byte) *Canon {
	if c.err != nil {
		return c
	}
	if uint64(len(b)) > math.MaxUint32 {
		c.err = ErrFieldTooLarge
		return c
	}
	c.buf = binary.BigEndian.AppendUint32(c.buf, uint32(len(b))) // #nosec G115 -- len(b) <= math.MaxUint32 is checked above
	c.buf = append(c.buf, b...)
	return c
}

// String appends the UTF-8 bytes of s as one field.
func (c *Canon) String(s string) *Canon { return c.Bytes([]byte(s)) }

// ID appends a 16-byte identifier as one field.
func (c *Canon) ID(id ID) *Canon { return c.Bytes(id[:]) }

// Hash appends a 32-byte digest as one field.
func (c *Canon) Hash(h [32]byte) *Canon { return c.Bytes(h[:]) }

// U16 appends a fixed-width big-endian integer field.
func (c *Canon) U16(v uint16) *Canon { return c.Bytes(binary.BigEndian.AppendUint16(nil, v)) }

// U32 appends a fixed-width big-endian integer field.
func (c *Canon) U32(v uint32) *Canon { return c.Bytes(binary.BigEndian.AppendUint32(nil, v)) }

// U64 appends a fixed-width big-endian integer field.
func (c *Canon) U64(v uint64) *Canon { return c.Bytes(binary.BigEndian.AppendUint64(nil, v)) }

// Bool appends a one-byte field, 1 for true and 0 for false.
func (c *Canon) Bool(v bool) *Canon {
	if v {
		return c.Bytes([]byte{1})
	}
	return c.Bytes([]byte{0})
}

// IDs appends the list length and then each identifier, so moving an element across a list
// boundary changes the encoding.
func (c *Canon) IDs(ids []ID) *Canon {
	c.U64(uint64(len(ids)))
	for _, id := range ids {
		c.ID(id)
	}
	return c
}

// Encoded returns a copy of the bytes built so far, or the first error.
func (c *Canon) Encoded() ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	return bytes.Clone(c.buf), nil
}

// Sum returns the SHA-256 of the bytes built so far, or the first error.
func (c *Canon) Sum() ([32]byte, error) {
	if c.err != nil {
		return [32]byte{}, c.err
	}
	return sha256.Sum256(c.buf), nil
}
