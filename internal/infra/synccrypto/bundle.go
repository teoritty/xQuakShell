package synccrypto

import (
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"xquakshell/internal/domain/vaultsync"
)

// AccountKeys are the two account private keys, in seed form, that the bundle carries between a
// user's devices: the identity key (AIK) and the encryption key (AEK).
type AccountKeys struct {
	IdentitySeed   []byte
	EncryptionSeed []byte
}

const seedSize = 32

// bundleKey derives the key that seals the bundle from the OPAQUE export key. The export key is
// computed on the client from the password and the server's OPRF answer and never leaves the
// client, so the server stores the bundle without any way to open it.
func bundleKey(exportKey []byte) ([]byte, error) {
	if len(exportKey) == 0 {
		return nil, fmt.Errorf("sync: empty export key")
	}
	return hkdf.Key(sha256.New, exportKey, nil, vaultsync.LabelBundle, keySize)
}

// SealBundle seals the account keys for storage on the server, bound to the account id so the
// server cannot serve one account's bundle to a login for another.
func SealBundle(rnd io.Reader, exportKey []byte, account vaultsync.ID, keys AccountKeys) ([]byte, error) {
	if len(keys.IdentitySeed) != seedSize || len(keys.EncryptionSeed) != seedSize {
		return nil, fmt.Errorf("sync: account key seeds must be %d bytes", seedSize)
	}
	key, err := bundleKey(exportKey)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	plain, err := vaultsync.NewCanon(vaultsync.LabelBundle).Bytes(keys.IdentitySeed).Bytes(keys.EncryptionSeed).Encoded()
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	aad, err := vaultsync.BundleAAD(account, vaultsync.Suite1)
	if err != nil {
		return nil, err
	}
	return seal(rnd, key, plain, aad)
}

// OpenBundle opens a bundle with the export key from a successful login. A wrong password produces
// a wrong export key and therefore the same ErrOpen as a tampered bundle.
func OpenBundle(exportKey []byte, account vaultsync.ID, sealed []byte) (AccountKeys, error) {
	key, err := bundleKey(exportKey)
	if err != nil {
		return AccountKeys{}, ErrOpen
	}
	defer clear(key)
	aad, err := vaultsync.BundleAAD(account, vaultsync.Suite1)
	if err != nil {
		return AccountKeys{}, ErrOpen
	}
	plain, err := open(key, sealed, aad)
	if err != nil {
		return AccountKeys{}, ErrOpen
	}
	defer clear(plain)
	return decodeBundle(plain)
}

// decodeBundle reads exactly the layout SealBundle writes - the label and two 32-byte seeds - and
// nothing else, so a bundle written by a newer layout is refused rather than half-read.
func decodeBundle(plain []byte) (AccountKeys, error) {
	rest := plain
	var fields [3][]byte
	for i := range fields {
		if len(rest) < 4 {
			return AccountKeys{}, ErrOpen
		}
		n := binary.BigEndian.Uint32(rest)
		if int64(n) > int64(len(rest)-4) {
			return AccountKeys{}, ErrOpen
		}
		fields[i], rest = rest[4:4+n], rest[4+n:]
	}
	if len(rest) != 0 || !bytes.Equal(fields[0], []byte(vaultsync.LabelBundle)) ||
		len(fields[1]) != seedSize || len(fields[2]) != seedSize {
		return AccountKeys{}, ErrOpen
	}
	return AccountKeys{IdentitySeed: bytes.Clone(fields[1]), EncryptionSeed: bytes.Clone(fields[2])}, nil
}
