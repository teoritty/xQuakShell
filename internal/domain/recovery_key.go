package domain

import "strings"

const (
	// RecoveryKeyBytes is the entropy behind one key. Twenty bytes is 160 bits, which is beyond any
	// offline search, and it divides by five exactly - so the base32 encoding lands on a whole
	// character count with no padding to explain to the user.
	RecoveryKeyBytes = 20

	// RecoveryKeyLength is how many characters a normalized key has. It follows from
	// RecoveryKeyBytes: eight bits per byte over five bits per character.
	RecoveryKeyLength = RecoveryKeyBytes * 8 / 5

	// RecoveryKeyGroupSize is how many characters sit between the dashes when a key is displayed.
	// Four is short enough to hold in your head while copying one group to paper.
	RecoveryKeyGroupSize = 4

	// recoveryKeyAlphabet is Crockford's base32: the digits plus the letters, minus I, L, O and U.
	// I and L are dropped because they are the digit one in most fonts, O because it is the digit
	// zero, and U so that no generated key spells an obscenity. The order is the encoding - index
	// into this string is the five-bit value.
	recoveryKeyAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// NormalizeRecoveryKey turns what a person typed into the canonical key, and reports whether the
// result is a well-formed one.
//
// Everything a human adds while copying from paper is discarded: lowercase, the display dashes, and
// any whitespace including the newline a paste from the downloaded file brings with it. The three
// confusable letters are folded to the digits they look like, which is the whole point of choosing
// Crockford - someone who read a zero as the letter O still gets in.
//
// The fold is deliberately one-way and lossy in the safe direction: I, L and O are not in the
// alphabet, so no generated key contains them, and nothing an attacker types can therefore reach a
// canonical key that a different input could not already reach.
func NormalizeRecoveryKey(input string) (string, bool) {
	var b strings.Builder
	b.Grow(len(input))

	for _, r := range strings.ToUpper(input) {
		switch r {
		case '-', ' ', '\t', '\n', '\r':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if !strings.ContainsRune(recoveryKeyAlphabet, r) {
			return "", false
		}
		b.WriteRune(r)
	}

	key := b.String()
	if len(key) != RecoveryKeyLength {
		return "", false
	}
	return key, true
}

// LooksLikeRecoveryKey reports whether input could be a key at all, without saying whether it is
// the right one.
//
// The unlock path uses this to decide which credential it was handed. That decision leaks nothing:
// it depends only on the characters the caller just typed, which the caller already knows.
func LooksLikeRecoveryKey(input string) bool {
	_, ok := NormalizeRecoveryKey(input)
	return ok
}

// FormatRecoveryKey inserts the display dashes into a canonical key, producing the grouped form the
// user is shown and asked to write down.
//
// A key that is not canonical is returned untouched rather than mangled into groups, so a caller
// that formats the wrong string sees the wrong string instead of something that looks right.
func FormatRecoveryKey(key string) string {
	if len(key) != RecoveryKeyLength {
		return key
	}

	var b strings.Builder
	b.Grow(RecoveryKeyLength + RecoveryKeyLength/RecoveryKeyGroupSize)
	for i := 0; i < RecoveryKeyLength; i += RecoveryKeyGroupSize {
		if i > 0 {
			b.WriteByte('-')
		}
		b.WriteString(key[i : i+RecoveryKeyGroupSize])
	}
	return b.String()
}

// EncodeRecoveryKey renders RecoveryKeyBytes of entropy as a canonical key.
//
// This is Crockford base32 written out rather than taken from encoding/base32, because the standard
// library only offers the RFC 4648 alphabet, and that alphabet contains exactly the characters this
// one exists to avoid. Input of any other length returns the empty string: a short key silently
// encoded would be a weak key nobody notices.
func EncodeRecoveryKey(entropy []byte) string {
	if len(entropy) != RecoveryKeyBytes {
		return ""
	}

	out := make([]byte, 0, RecoveryKeyLength)
	var acc uint16
	var bits uint

	for _, by := range entropy {
		acc = acc<<8 | uint16(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out = append(out, recoveryKeyAlphabet[(acc>>bits)&0x1f])
		}
	}
	// RecoveryKeyBytes*8 is a multiple of five, so bits is zero here and there is no remainder to
	// flush. The guard above is what keeps that true.
	return string(out)
}
