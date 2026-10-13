package usecase

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrSyncAccount is a stored sync account with a missing or malformed key.
	ErrSyncAccount = errors.New("sync: the stored sync account is incomplete")
	// ErrNoVaultKey is a vault epoch whose membership log carries no key for this account.
	ErrNoVaultKey = errors.New("sync: no vault key for this account at that epoch")
)

// syncKeyRing is the KeyRing over the account stored in the vault.
type syncKeyRing struct {
	account, device vaultsync.ID
	deviceKey       ed25519.PrivateKey
	identityKey     []byte
	encSeed, encKey []byte
	crypto          synctrust.AccountCrypto
}

// NewSyncKeyRing reads the device's keys from the stored account, refusing an account with any
// key of the wrong size rather than failing later in the middle of a commit.
func NewSyncKeyRing(a *domain.SyncAccount, crypto synctrust.AccountCrypto) (synctrust.KeyRing, error) {
	if a == nil || len(a.Account) != len(vaultsync.ID{}) || len(a.Device) != len(vaultsync.ID{}) ||
		len(a.DeviceSeed) != ed25519.SeedSize || len(a.IdentitySeed) != ed25519.SeedSize ||
		len(a.EncryptionSeed) == 0 || len(a.EncryptionKey) == 0 {
		return nil, ErrSyncAccount
	}
	k := &syncKeyRing{deviceKey: ed25519.NewKeyFromSeed(a.DeviceSeed), encSeed: bytes.Clone(a.EncryptionSeed),
		encKey: bytes.Clone(a.EncryptionKey), crypto: crypto}
	copy(k.account[:], a.Account)
	copy(k.device[:], a.Device)
	k.identityKey = ed25519.NewKeyFromSeed(a.IdentitySeed).Public().(ed25519.PublicKey)
	return k, nil
}

func (k *syncKeyRing) Account() vaultsync.ID { return k.account }
func (k *syncKeyRing) Device() vaultsync.ID  { return k.device }

func (k *syncKeyRing) AccountKeys() (vaultsync.SuiteID, []byte, []byte) {
	return vaultsync.Suite1, bytes.Clone(k.identityKey), bytes.Clone(k.encKey)
}

func (k *syncKeyRing) Sign(msg []byte) ([]byte, error) { return ed25519.Sign(k.deviceKey, msg), nil }

// VaultKey unwraps the vault key of one epoch from the latest verified op that wraps it for this
// account. The ops come from a replayed membership log, and the wrap is bound to vault, epoch and
// member, so a wrap moved from elsewhere does not open.
func (k *syncKeyRing) VaultKey(vault vaultsync.ID, epoch uint32, ops []vaultsync.MemberOp) ([]byte, error) {
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i].Epoch != epoch {
			continue
		}
		if wrapped, ok := k.wrapIn(ops[i]); ok {
			vk, err := k.crypto.UnwrapVaultKey(k.encSeed, vault, epoch, k.account, wrapped)
			if err != nil {
				return nil, fmt.Errorf("unwrap vault key: %w", err)
			}
			return vk, nil
		}
	}
	return nil, ErrNoVaultKey
}

func (k *syncKeyRing) wrapIn(op vaultsync.MemberOp) ([]byte, bool) {
	for _, w := range op.Wraps {
		if w.Member == k.account {
			return w.Ciphertext, true
		}
	}
	return nil, false
}
