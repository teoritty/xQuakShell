package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"

	"xquakshell/internal/domain"
)

// scryptWorkFactor sets scrypt's cost parameter to log2N=18 — the minimum value
// permitted by the age v1 spec for the scrypt recipient stanza. This is a
// deliberate security/performance tradeoff, not an arbitrary constant:
//
//   - scrypt's memory-hardness is the entire reason it's used here instead of a
//     cheaper KDF: deriving a wrapping key from the master password is made
//     expensive in RAM as well as CPU, which is what makes offline brute-forcing
//     of the master password expensive on GPUs/ASICs (those gain little from
//     extra RAM-per-core the way they do from raw compute).
//   - At N=2^18 with r=8 (r is fixed by the age spec; only log2N is configurable
//     here), scrypt's working buffer is 128*N*r bytes, i.e. ~256 MiB. That memory
//     is allocated transiently on every wrap and unwrap. It is freed immediately
//     after the call returns — it is NOT a leak — but the Go runtime's background
//     scavenger can take several minutes to hand the underlying OS pages back,
//     which shows up as a long-lingering RSS spike if nothing forces an earlier
//     release. See the runtime.GC()/debug.FreeOSMemory() calls in
//     internal/infra/persistence/vault_repo.go for where that's handled.
//   - Do not lower this value to reduce memory usage. log2N=18 is already the
//     floor the age spec allows; going lower weakens master-password
//     brute-force resistance. If the ~256 MiB transient cost is genuinely a
//     problem on a constrained target, that's a deliberate product decision to
//     revisit (e.g. a configurable work factor with a documented security
//     tradeoff), not a default to quietly tune down.
//
// Since the envelope format, this cost is paid once per credential wrap rather
// than on every write of the vault: the payload is encrypted to a vault key that
// needs no derivation at all. The protection is unchanged — an attacker still
// has to run scrypt per password guess — while a routine flush no longer does.
const scryptWorkFactor = 18

// EncryptLegacy produces the pre-envelope vault format: the data encrypted directly under a
// password-derived scrypt key, with no wraps and no vault key.
//
// Nothing in the application writes this any more. It survives because the migration path needs to
// build one to test against, and because reading a file this function produced is exactly what
// every existing installation asks for on its next unlock.
func EncryptLegacy(data *domain.VaultData, passphrase string) ([]byte, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("vault marshal: %w", err)
	}

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, fmt.Errorf("vault scrypt recipient: %w", err)
	}
	recipient.SetWorkFactor(scryptWorkFactor)

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("vault encrypt init: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("vault encrypt write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("vault encrypt close: %w", err)
	}
	return buf.Bytes(), nil
}

// DecryptLegacy opens a pre-envelope vault with the master password.
//
// It runs the same scrypt derivation as EncryptLegacy and has the same ~256 MiB transient memory
// cost — see scryptWorkFactor. Callers that care about RSS settling quickly should force a GC pass
// afterward (see VaultRepo.Unlock).
func DecryptLegacy(ciphertext []byte, passphrase string) (*domain.VaultData, error) {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("vault scrypt identity: %w", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("vault decrypt: %w", domain.ErrVaultDecryptFailed)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("vault decrypt read: %w", domain.ErrVaultDecryptFailed)
	}
	return decodeVaultData(plaintext)
}

// decodeVaultData parses decrypted vault JSON and applies the schema version gates.
//
// A version between the migratable floor and the current one is returned as it is, carrying its own
// Version field. This deliberately does not upgrade it: a migration rewrites the file, and doing
// that from inside a function every read path calls would mean an unlock that merely inspects the
// vault could rewrite it. The caller that can take a backup first is the one allowed to migrate.
func decodeVaultData(plaintext []byte) (*domain.VaultData, error) {
	var data domain.VaultData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("vault unmarshal: %w", err)
	}
	if data.Version > domain.CurrentVaultVersion {
		return nil, fmt.Errorf("vault version %d, this build reads %d: %w",
			data.Version, domain.CurrentVaultVersion, domain.ErrVaultVersionTooNew)
	}
	if data.Version < domain.MinMigratableVaultVersion {
		return nil, fmt.Errorf("vault version %d, this build migrates from %d: %w",
			data.Version, domain.MinMigratableVaultVersion, domain.ErrVaultVersionTooOld)
	}
	return &data, nil
}

// NeedsMigration reports whether decrypted data predates the current schema.
func NeedsMigration(data *domain.VaultData) bool {
	return data != nil && data.Version < domain.CurrentVaultVersion
}
