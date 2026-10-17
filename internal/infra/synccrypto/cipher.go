package synccrypto

import (
	"crypto/rand"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// Cipher is SealRecord and OpenRecord behind synctrust.RecordCipher, with the coordinates taken from
// the envelope itself so a caller cannot seal under one set and file the result under another.
type Cipher struct{}

var _ synctrust.RecordCipher = Cipher{}

func coordsOf(e *vaultsync.Envelope) RecordCoords {
	return RecordCoords{Vault: e.Vault, Record: e.Record, Version: e.Version, Epoch: e.Epoch, Suite: e.Suite}
}

// Seal fills e.WrappedDEK and e.Body under a fresh data key from the system's randomness.
func (Cipher) Seal(vaultKey []byte, e *vaultsync.Envelope, plaintext []byte) error {
	dek, body, err := SealRecord(rand.Reader, vaultKey, coordsOf(e), plaintext)
	if err != nil {
		return err
	}
	e.WrappedDEK, e.Body = dek, body
	return nil
}

// Open fails unless every coordinate matches the seal: a body moved to another record, version or
// epoch does not authenticate.
func (Cipher) Open(vaultKey []byte, e *vaultsync.Envelope) ([]byte, error) {
	return OpenRecord(vaultKey, coordsOf(e), e.WrappedDEK, e.Body)
}
