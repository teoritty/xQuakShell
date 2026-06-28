package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"

	"ssh-client/internal/domain"
)

// Encrypt serializes data to JSON and encrypts it with the given passphrase using age scrypt.
// Returns the encrypted ciphertext or an error.
func Encrypt(data *domain.VaultData, passphrase string) ([]byte, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("vault marshal: %w", err)
	}

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, fmt.Errorf("vault scrypt recipient: %w", err)
	}
	// SetWorkFactor(18) sets scrypt's cost parameter to log2N=18 — the minimum
	// value permitted by the age v1 spec for the scrypt recipient stanza. This
	// is a deliberate security/performance tradeoff, not an arbitrary constant:
	//
	//   - scrypt's memory-hardness is the entire reason it's used here instead
	//     of a cheaper KDF: deriving the vault key from the master password is
	//     made expensive in RAM as well as CPU, which is what makes offline
	//     brute-forcing of the master password expensive on GPUs/ASICs (those
	//     gain little from extra RAM-per-core the way they do from raw compute).
	//   - At N=2^18 with r=8 (r is fixed by the age spec; only log2N is
	//     configurable here), scrypt's working buffer is 128*N*r bytes, i.e.
	//     ~256 MiB. That memory is allocated transiently on every Encrypt() and
	//     every Decrypt() call. It is freed immediately after the call returns —
	//     it is NOT a leak — but the Go runtime's background scavenger can take
	//     several minutes to hand the underlying OS pages back, which shows up
	//     as a long-lingering RSS spike if nothing forces an earlier release.
	//     See the runtime.GC()/debug.FreeOSMemory() calls in
	//     internal/infra/persistence/vault_repo.go (both Unlock and flushNow)
	//     for where that's handled.
	//   - Do not lower this value to reduce memory usage. log2N=18 is already
	//     the floor the age spec allows; going lower weakens master-password
	//     brute-force resistance. If the ~256 MiB transient cost is genuinely a
	//     problem on a constrained target, that's a deliberate product decision
	//     to revisit (e.g. a configurable work factor with a documented security
	//     tradeoff), not a default to quietly tune down.
	recipient.SetWorkFactor(18)

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

// Decrypt decrypts ciphertext with the given passphrase and deserializes the JSON payload.
// Returns ErrVaultDecryptFailed if the passphrase is wrong or data is corrupted.
//
// Decrypt runs the same scrypt key derivation as Encrypt and has the same
// ~256 MiB transient memory cost — see the comment on SetWorkFactor in this
// file for why. Callers that care about RSS settling quickly after a call to
// Decrypt should force a GC pass afterward (see VaultRepo.Unlock).
func Decrypt(ciphertext []byte, passphrase string) (*domain.VaultData, error) {
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

	var versionProbe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(plaintext, &versionProbe); err != nil {
		return nil, fmt.Errorf("vault unmarshal version: %w", err)
	}

	var data *domain.VaultData
	if versionProbe.Version < domain.CurrentVaultVersion {
		legacy, err := unmarshalVaultLegacy(plaintext)
		if err != nil {
			return nil, fmt.Errorf("vault unmarshal legacy: %w", err)
		}
		data = legacy
	} else {
		var current domain.VaultData
		if err := json.Unmarshal(plaintext, &current); err != nil {
			return nil, fmt.Errorf("vault unmarshal: %w", err)
		}
		data = &current
	}

	return data, nil
}
