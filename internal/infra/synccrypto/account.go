package synccrypto

import (
	"crypto/rand"
	"fmt"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// Account is the account-key functions of this package behind synctrust.AccountCrypto.
type Account struct{}

var _ synctrust.AccountCrypto = Account{}

// GenerateEncryptionKey creates an account encryption key.
func (Account) GenerateEncryptionKey() ([]byte, []byte, error) { return GenerateEncryptionKey() }

// EncryptionPublic derives the public key of an encryption seed, so a device can check that the
// bundle it opened holds the key the server and the device log advertise.
func (Account) EncryptionPublic(seed []byte) ([]byte, error) {
	priv, err := suite1KEM().NewPrivateKey(seed)
	if err != nil {
		return nil, fmt.Errorf("sync: encryption seed: %w", err)
	}
	return priv.PublicKey().Bytes(), nil
}

// SealBundle seals the account keys under the export key with fresh randomness.
func (Account) SealBundle(exportKey []byte, account vaultsync.ID, identitySeed, encryptionSeed []byte) ([]byte, error) {
	return SealBundle(rand.Reader, exportKey, account, AccountKeys{IdentitySeed: identitySeed, EncryptionSeed: encryptionSeed})
}

// OpenBundle opens a bundle sealed for this account under this export key.
func (Account) OpenBundle(exportKey []byte, account vaultsync.ID, sealed []byte) ([]byte, []byte, error) {
	k, err := OpenBundle(exportKey, account, sealed)
	return k.IdentitySeed, k.EncryptionSeed, err
}

// WrapVaultKey seals a vault key to one member for one vault and epoch.
func (Account) WrapVaultKey(memberPublic []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, vk []byte) ([]byte, error) {
	return WrapVaultKey(memberPublic, vault, epoch, member, vk)
}

// UnwrapVaultKey opens a wrap made for exactly this vault, epoch and member.
func (Account) UnwrapVaultKey(seed []byte, vault vaultsync.ID, epoch uint32, member vaultsync.ID, wrapped []byte) ([]byte, error) {
	return UnwrapVaultKey(seed, vault, epoch, member, wrapped)
}
