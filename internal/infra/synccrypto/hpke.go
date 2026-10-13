package synccrypto

import (
	"crypto/hpke"
	"fmt"

	"xquakshell/internal/domain/vaultsync"
)

// The HPKE choices of suite 1. The hybrid KEM means a vault key recorded today stays safe against
// a future quantum computer as long as either ML-KEM-768 or X25519 holds.
func suite1KEM() hpke.KEM   { return hpke.MLKEM768X25519() }
func suite1KDF() hpke.KDF   { return hpke.HKDFSHA256() }
func suite1AEAD() hpke.AEAD { return hpke.ChaCha20Poly1305() }

// GenerateEncryptionKey creates an account encryption key (AEK). The 32-byte seed is the private
// key in its storage form; the public key is what vault keys are wrapped to.
func GenerateEncryptionKey() (seed, public []byte, err error) {
	priv, err := suite1KEM().GenerateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("sync: generate encryption key: %w", err)
	}
	if seed, err = priv.Bytes(); err != nil {
		return nil, nil, fmt.Errorf("sync: export encryption key: %w", err)
	}
	return seed, priv.PublicKey().Bytes(), nil
}

// WrapVaultKey seals a vault key to one member for one vault and epoch. The HPKE info carries all
// three, so the server cannot hand one member's wrap to another or replay it for another epoch.
func WrapVaultKey(memberPublic []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, vk []byte) ([]byte, error) {
	if len(vk) != keySize {
		return nil, fmt.Errorf("sync: vault key is %d bytes, want %d", len(vk), keySize)
	}
	pub, err := suite1KEM().NewPublicKey(memberPublic)
	if err != nil {
		return nil, fmt.Errorf("sync: member encryption key: %w", err)
	}
	info, err := vaultsync.VKWrapInfo(vault, epoch, member)
	if err != nil {
		return nil, err
	}
	return hpke.Seal(pub, suite1KDF(), suite1AEAD(), info, vk)
}

// UnwrapVaultKey opens a wrap with the member's seed, refusing any wrap not made for exactly this
// vault, epoch and member, and any size suite 1 cannot produce.
func UnwrapVaultKey(seed []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, wrapped []byte) ([]byte, error) {
	p, err := vaultsync.LookupSuite(vaultsync.Suite1, vaultsync.MinAcceptedSuite)
	if err != nil || len(wrapped) != p.VKWrapSize {
		return nil, ErrOpen
	}
	priv, err := suite1KEM().NewPrivateKey(seed)
	if err != nil {
		return nil, ErrOpen
	}
	info, err := vaultsync.VKWrapInfo(vault, epoch, member)
	if err != nil {
		return nil, ErrOpen
	}
	vk, err := hpke.Open(priv, suite1KDF(), suite1AEAD(), info, wrapped)
	if err != nil || len(vk) != keySize {
		return nil, ErrOpen
	}
	return vk, nil
}
