// Package synccrypto implements the cryptography of sync suite 1 (ADR-023): record sealing under
// fresh data keys, hybrid post-quantum wrapping of vault keys, account key bundles and signing
// keys. Every function takes the coordinates a value is bound to and builds its additional data
// with the vaultsync builders, so a caller cannot seal something without binding it.
//
// It is shared by the application and the sync server, which must compute these identically.
package synccrypto

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"

	"xquakshell/internal/domain/vaultsync"
)

// ErrOpen is the single error for every failure to open sealed data. Distinguishing a bad key from
// a bad tag from a bad length would hand whoever controls the ciphertext an oracle.
var ErrOpen = errors.New("sync: sealed data does not open")

// RecordCoords is where a record version lives. Both the body and its data key are bound to all
// of it, so a ciphertext the server moves anywhere else fails to open.
type RecordCoords struct {
	Vault   vaultsync.ID
	Record  vaultsync.ID
	Version uint64
	Epoch   uint32
	Suite   vaultsync.SuiteID
}

const keySize = chacha20poly1305.KeySize

// SealRecord pads and seals one record version under a fresh data key, and wraps that key under
// the vault key. The data key is never reused: a second version gets a second key, which is what
// makes nonce reuse impossible and lets one version be shredded alone.
func SealRecord(rnd io.Reader, vk []byte, c RecordCoords, plaintext []byte) (wrappedDEK, body []byte, err error) {
	if len(vk) != keySize {
		return nil, nil, fmt.Errorf("sync: vault key is %d bytes, want %d", len(vk), keySize)
	}
	padded, err := vaultsync.Pad(plaintext)
	if err != nil {
		return nil, nil, err
	}
	dek := make([]byte, keySize)
	defer clear(dek)
	if _, err := io.ReadFull(rnd, dek); err != nil {
		return nil, nil, fmt.Errorf("sync: draw data key: %w", err)
	}
	bodyAAD, err := vaultsync.BodyAAD(c.Vault, c.Record, c.Version, c.Epoch, c.Suite)
	if err != nil {
		return nil, nil, err
	}
	if body, err = seal(rnd, dek, padded, bodyAAD); err != nil {
		return nil, nil, err
	}
	wrapAAD, err := vaultsync.DEKWrapAAD(c.Vault, c.Record, c.Version, c.Epoch)
	if err != nil {
		return nil, nil, err
	}
	if wrappedDEK, err = seal(rnd, vk, dek, wrapAAD); err != nil {
		return nil, nil, err
	}
	return wrappedDEK, body, nil
}

// OpenRecord reverses SealRecord, refusing every size SealRecord cannot have produced before any
// cipher runs.
func OpenRecord(vk []byte, c RecordCoords, wrappedDEK, body []byte) ([]byte, error) {
	params, err := vaultsync.LookupSuite(c.Suite, vaultsync.MinAcceptedSuite)
	if err != nil || len(vk) != keySize || len(wrappedDEK) != params.WrappedDEKSize {
		return nil, ErrOpen
	}
	wrapAAD, err := vaultsync.DEKWrapAAD(c.Vault, c.Record, c.Version, c.Epoch)
	if err != nil {
		return nil, ErrOpen
	}
	dek, err := open(vk, wrappedDEK, wrapAAD)
	if err != nil {
		return nil, ErrOpen
	}
	defer clear(dek)
	bodyAAD, err := vaultsync.BodyAAD(c.Vault, c.Record, c.Version, c.Epoch, c.Suite)
	if err != nil {
		return nil, ErrOpen
	}
	padded, err := open(dek, body, bodyAAD)
	if err != nil {
		return nil, ErrOpen
	}
	plaintext, err := vaultsync.Unpad(padded)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}

// seal is XChaCha20-Poly1305 with a random 192-bit nonce prefixed to the ciphertext. At that nonce
// size random nonces are safe for any number of messages a vault will ever see.
func seal(rnd io.Reader, key, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(plaintext)+aead.Overhead())
	if _, err := io.ReadFull(rnd, nonce); err != nil {
		return nil, fmt.Errorf("sync: draw nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, sealed, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrOpen
	}
	n := aead.NonceSize()
	return aead.Open(nil, sealed[:n], sealed[n:], aad)
}
