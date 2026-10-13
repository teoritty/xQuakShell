package vaultsync

import (
	"bytes"
	"encoding/binary"
	"math/bits"
)

const (
	// MinPaddedSize hides every small record - a folder, a password, a setting - behind one size.
	MinPaddedSize = 256
	// MaxPlaintextSize bounds a record: a 16 MiB plugin file base64-encoded inside its JSON body
	// fits with room to spare.
	MaxPlaintextSize = 32 << 20

	lengthPrefix = 4
)

// padme rounds n up with the Padmé scheme (Nikitin et al., 2019): sizes leak O(log log n) bits
// instead of O(log n), for at most about 12% overhead.
func padme(n int) int {
	if n < 2 {
		return n
	}
	e := bits.Len(uint(n)) - 1
	s := bits.Len(uint(e))
	mask := (1 << (e - s)) - 1
	return (n + mask) &^ mask
}

// PaddedSize is the padded length of a plaintext of n bytes.
func PaddedSize(n int) int {
	return max(MinPaddedSize, padme(lengthPrefix+n))
}

// Pad prefixes the plaintext with its length and zero-fills to PaddedSize, so the server cannot
// tell a password from a folder by the size of the ciphertext.
func Pad(plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxPlaintextSize {
		return nil, ErrPlaintextTooLarge
	}
	out := make([]byte, PaddedSize(len(plaintext)))
	binary.BigEndian.PutUint32(out, uint32(len(plaintext))) // #nosec G115 -- bounded by MaxPlaintextSize above
	copy(out[lengthPrefix:], plaintext)
	return out, nil
}

// Unpad reverses Pad and accepts only its exact output: the right length, an all-zero tail, and
// the canonical size for that length. Anything looser would let two different byte strings decode
// to one plaintext.
func Unpad(padded []byte) ([]byte, error) {
	if len(padded) < MinPaddedSize {
		return nil, ErrBadPadding
	}
	n := binary.BigEndian.Uint32(padded)
	if uint64(n) > uint64(len(padded)-lengthPrefix) || len(padded) != PaddedSize(int(n)) {
		return nil, ErrBadPadding
	}
	end := lengthPrefix + int(n)
	for _, b := range padded[end:] {
		if b != 0 {
			return nil, ErrBadPadding
		}
	}
	return bytes.Clone(padded[lengthPrefix:end]), nil
}
