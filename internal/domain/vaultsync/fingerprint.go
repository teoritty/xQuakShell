package vaultsync

import (
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"strings"
)

// AccountFingerprint renders an account identity key as twelve groups of five digits for people to
// compare out of band. It is computed over the account's genesis identity key, which never changes:
// later keys are trusted through the signed succession chain back to it, so a fingerprint compared
// once stays valid across key rotations.
//
// Each group is five bytes of SHA-512 reduced modulo 100000; the bias that reduction introduces is
// below one part in ten million per group, far under what a person comparing digits would notice.
func AccountFingerprint(identityKey []byte) string {
	sum := sha512.Sum512(identityKey)
	groups := make([]string, 12)
	for i := range groups {
		var chunk [8]byte
		copy(chunk[3:], sum[i*5:i*5+5])
		groups[i] = fmt.Sprintf("%05d", binary.BigEndian.Uint64(chunk[:])%100000)
	}
	return strings.Join(groups, " ")
}
