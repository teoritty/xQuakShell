package usecase

import (
	"context"
	"crypto/rand"
	"fmt"

	"xquakshell/internal/domain"
)

// The replication key uses the recovery key's format, and the helpers that go with it.
//
// The two credentials do different jobs but have the same problem: a person has to move a secret
// off one screen and into another machine, by hand, without a channel to send it over. That is what
// the Crockford base32 alphabet, the display grouping and the confusable folding were chosen for,
// and a second format would mean a second set of transcription mistakes to design around for no
// gain. Reused rather than renamed: the helpers are generic, and a rename across a security path
// would be churn.

// MintReplicaKey issues the key that seals this plugin's replicas, and returns it once so the user
// can carry it to their other devices (ADR-022).
//
// It refuses to replace a key that exists. Everything sealed under the old one would become
// unreadable, and the user would find out only on the next fetch. Joining an existing set of
// devices goes through ImportReplicaKey, where the user has the key in hand and means it.
func (s *PluginVaultSettings) MintReplicaKey(ctx context.Context, pluginID string) (string, error) {
	if s == nil || s.vault == nil || pluginID == "" {
		return "", fmt.Errorf("mint replication key: no vault")
	}
	entropy := make([]byte, domain.RecoveryKeyBytes)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("mint replication key: %w", err)
	}
	key := domain.EncodeRecoveryKey(entropy)
	if key == "" {
		return "", fmt.Errorf("mint replication key: encoding produced nothing")
	}
	// The key is returned only after the write lands. Handing back a key the vault does not hold
	// would have the user copy it to a second device that the first has no record of.
	if err := s.storeReplicaKey(ctx, pluginID, key, false); err != nil {
		return "", err
	}
	return key, nil
}

// ImportReplicaKey accepts the key from another of the user's devices.
//
// Everything a person adds while copying from paper - lowercase, the display dashes, the newline a
// paste brings with it - is discarded before the key is compared or stored, so the ordinary way of
// copying a key works. Unlike minting, this does replace what is there: it is the explicit act of
// joining devices, with the key in hand.
func (s *PluginVaultSettings) ImportReplicaKey(ctx context.Context, pluginID, typed string) error {
	if s == nil || s.vault == nil || pluginID == "" {
		return fmt.Errorf("import replication key: no vault")
	}
	key, ok := domain.NormalizeRecoveryKey(typed)
	if !ok {
		return fmt.Errorf("import replication key for %s: %w", pluginID, domain.ErrReplicaKeyMalformed)
	}
	return s.storeReplicaKey(ctx, pluginID, key, true)
}

// ReplicaKey returns the key this plugin's replicas are sealed with.
//
// A plugin with no key has not been set up for replication yet, which is an ordinary state and not
// a failure - so it comes back as ErrReplicaKeyRequired rather than as an unreadable vault error.
func (s *PluginVaultSettings) ReplicaKey(_ context.Context, pluginID string) (string, error) {
	if s == nil || s.vault == nil {
		return "", fmt.Errorf("read replication key: no vault")
	}
	data, err := s.vault.GetData()
	if err != nil {
		return "", fmt.Errorf("read replication key for %s: %w", pluginID, err)
	}
	key := data.ReplicaKeys[pluginID]
	if key == "" {
		return "", fmt.Errorf("read replication key for %s: %w", pluginID, domain.ErrReplicaKeyRequired)
	}
	return key, nil
}

// forgetReplicaKeyLocked drops one plugin's key inside an open vault write, and reports whether
// there was one to drop.
//
// It runs as part of the uninstall write rather than as a call of its own, so there is no window in
// which the grants are gone and the key that opens the plugin's replicas is still there.
func forgetReplicaKeyLocked(data *domain.VaultData, pluginID string) bool {
	if _, held := data.ReplicaKeys[pluginID]; !held {
		return false
	}
	delete(data.ReplicaKeys, pluginID)
	return true
}

// storeReplicaKey writes one key, optionally over an existing one.
func (s *PluginVaultSettings) storeReplicaKey(ctx context.Context, pluginID, key string, replace bool) error {
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.ReplicaKeys == nil {
			data.ReplicaKeys = map[string]string{}
		}
		if !replace && data.ReplicaKeys[pluginID] != "" {
			return fmt.Errorf("replication key for %s: %w", pluginID, domain.ErrReplicaKeyExists)
		}
		data.ReplicaKeys[pluginID] = key
		return nil
	})
}
