package vault

import (
	"errors"
	"fmt"

	"xquakshell/internal/domain"
)

// PendingKey names a v3 identity whose passphrase the migration needs before it can rewrite the
// key. Only the metadata a user needs to recognise the key is carried; nothing here is secret.
type PendingKey struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
	KeyType string `json:"keyType"`
}

// MigrationReport records what a migration did, so the UI can tell the user which keys still need
// attention instead of reporting a bare success.
type MigrationReport struct {
	FromVersion int      `json:"fromVersion"`
	ToVersion   int      `json:"toVersion"`
	Converted   []string `json:"converted"`
	Skipped     []string `json:"skipped"`
	BackupPath  string   `json:"backupPath"`
}

// PlanMigration lists the identities whose passphrase the user must supply, without changing
// anything. It exists so the wizard can ask for every passphrase in one pass rather than
// interrupting a half-finished rewrite.
//
// The answer comes from the stored bytes through codec.Describe, not from the identity's own
// Encrypted flag: that flag was set by v3's header sniffing, which reported "openssh" for every
// modern key and could not see whether one was protected. Trusting it would put unprotected keys
// in the wizard and leave protected ones out of it.
func PlanMigration(data *domain.VaultData, codec domain.KeyCodec) []PendingKey {
	if !NeedsMigration(data) {
		return nil
	}
	pending := make([]PendingKey, 0)
	for id, identity := range data.Identities {
		blob, ok := data.KeyBlobs[id]
		if !ok || identity.Policy != "" && !blob.Legacy {
			continue
		}
		if _, encrypted := codec.Describe(blob.PEMData); !encrypted {
			continue
		}
		pending = append(pending, PendingKey{ID: id, Comment: identity.Comment, KeyType: identity.KeyType})
	}
	return pending
}

// MigrateToCurrent upgrades decrypted v3 data in place to the current schema.
//
// answers maps identity ID to the passphrase the user supplied; an ID absent from the map is
// skipped and keeps its original bytes under MigrationPending. Skipping is what stops a forgotten
// passphrase from making the whole vault unopenable — the alternative loses the user's
// connections and known hosts along with the one key they cannot remember.
//
// It returns an error only for conditions that must abort the whole upgrade. A key whose supplied
// passphrase turns out to be wrong is recorded as skipped rather than failing the migration: the
// user can retry it afterwards from the key manager, and refusing to open the vault over it would
// be the same dead end.
func MigrateToCurrent(data *domain.VaultData, codec domain.KeyCodec, newDataKey func() ([]byte, error), answers map[string]string) (*MigrationReport, error) {
	if data == nil {
		return nil, domain.ErrVaultNotFound
	}
	if !NeedsMigration(data) {
		return &MigrationReport{FromVersion: data.Version, ToVersion: data.Version}, nil
	}

	report := &MigrationReport{FromVersion: data.Version, ToVersion: domain.CurrentVaultVersion}
	for id := range data.Identities {
		identity := data.Identities[id]
		blob := data.KeyBlobs[id]
		converted, err := convertIdentity(&identity, &blob, codec, newDataKey, answers[id])
		if err != nil {
			return nil, err
		}
		if converted {
			report.Converted = append(report.Converted, id)
		} else {
			report.Skipped = append(report.Skipped, id)
		}
		data.Identities[id] = identity
		data.KeyBlobs[id] = blob
	}

	data.Version = domain.CurrentVaultVersion
	return report, nil
}

// convertIdentity rewrites one v3 key into the schema 4 shape, reporting whether it succeeded.
// A key it cannot convert is marked pending and left byte-for-byte as it was, so it still
// authenticates exactly as it did before the upgrade.
func convertIdentity(
	identity *domain.SSHIdentity,
	blob *domain.IdentityBlob,
	codec domain.KeyCodec,
	newDataKey func() ([]byte, error),
	passphrase string,
) (bool, error) {
	fillDefaults(identity)
	if identity.Policy != "" && !blob.Legacy {
		return true, nil
	}

	dataKey, err := newDataKey()
	if err != nil {
		return false, fmt.Errorf("migrate identity %s: %w", identity.ID, err)
	}
	// The key is re-wrapped under whatever will open it afterwards: the user's own passphrase
	// when they have one, the random data key when they do not. Re-wrapping a protected key
	// under the data key instead would move it out of the user's control and into the vault's,
	// which is the opposite of what they chose when they set a passphrase on it.
	wrapWith := []byte(passphrase)
	if passphrase == "" {
		wrapWith = dataKey
	}
	material, err := codec.Normalize(blob.PEMData, []byte(passphrase), wrapWith, identity.Comment)
	if err != nil {
		if errors.Is(err, domain.ErrPassphraseRequired) || errors.Is(err, domain.ErrKeyPassphraseWrong) {
			markPending(identity, blob)
			return false, nil
		}
		// A key that will not parse at all is not a migration failure to abort on: it did not
		// work before the upgrade either, and refusing to open the vault would strand every
		// other key beside it.
		markPending(identity, blob)
		return false, nil
	}

	identity.Policy = policyFor(passphrase)
	identity.Encrypted = passphrase != ""
	identity.KeyType = material.KeyType
	identity.Bits = material.Bits
	identity.PublicKey = material.PublicKey
	identity.Fingerprint = material.Fingerprint
	identity.MigrationPending = false
	blob.PEMData = material.PEM
	blob.Legacy = false
	blob.DataKey = dataKeyFor(passphrase, dataKey)
	return true, nil
}

// policyFor keeps a key that had its own passphrase under the user's control, and puts one that
// had none under the vault's. Migration never strengthens or weakens what the user chose.
func policyFor(passphrase string) domain.KeyPolicy {
	if passphrase != "" {
		return domain.KeyPolicyPassphrase
	}
	return domain.KeyPolicyVault
}

// dataKeyFor stores the random key only for a vault-policy identity. Storing it for a
// passphrase-policy one would defeat the policy entirely: the vault would then hold something
// that opens the key without the user.
func dataKeyFor(passphrase string, dataKey []byte) []byte {
	if passphrase != "" {
		return nil
	}
	return dataKey
}

func markPending(identity *domain.SSHIdentity, blob *domain.IdentityBlob) {
	identity.Policy = domain.KeyPolicyPassphrase
	identity.Encrypted = true
	identity.MigrationPending = true
	blob.Legacy = true
	blob.DataKey = nil
}

// fillDefaults gives a v3 identity the fields schema 4 expects. CacheUntilLock is the default
// because it is what the application did before caching was configurable, and an upgrade must not
// quietly change how often the user is asked for a passphrase.
func fillDefaults(identity *domain.SSHIdentity) {
	if identity.Cache == "" {
		identity.Cache = domain.CacheUntilLock
	}
	if identity.Source == "" {
		identity.Source = domain.SourceImported
	}
}
