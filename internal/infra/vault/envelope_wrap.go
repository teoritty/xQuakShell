package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"

	"xquakshell/internal/domain"
)

// newVaultKey mints the key the payload is encrypted to.
//
// It is an X25519 identity rather than a passphrase because nothing about it needs stretching: it
// is 256 bits of machine-generated entropy that no human ever sees or types. The expensive scrypt
// pass belongs on the credentials that wrap it, where the secret is a password someone chose.
func newVaultKey() (*age.X25519Identity, error) {
	key, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("vault key generate: %w", err)
	}
	return key, nil
}

// wrapVaultKey encrypts the vault key under a credential, producing one envelope wrap.
//
// This is where the master password's cost is paid, at the same scrypt work factor the whole vault
// used to be encrypted at - see the comment on SetWorkFactor in vault.go for why 18 and why it must
// not be lowered. Moving the derivation here changed which bytes it protects, never how hard it is.
func wrapVaultKey(key *age.X25519Identity, credential string) ([]byte, error) {
	recipient, err := age.NewScryptRecipient(credential)
	if err != nil {
		return nil, fmt.Errorf("vault wrap recipient: %w", err)
	}
	recipient.SetWorkFactor(scryptWorkFactor)

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("vault wrap init: %w", err)
	}
	if _, err := io.WriteString(writer, key.String()); err != nil {
		return nil, fmt.Errorf("vault wrap write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("vault wrap close: %w", err)
	}
	return buf.Bytes(), nil
}

// unwrapVaultKey recovers the vault key from a wrap, given the credential that made it.
//
// Every failure returns domain.ErrVaultDecryptFailed with no detail about which step failed. A wrap
// that decrypts to bytes that are not an identity means the file was tampered with, not that the
// credential was close - and saying so would tell a guesser they had found a real wrap.
func unwrapVaultKey(wrapped []byte, credential string) (*age.X25519Identity, error) {
	identity, err := age.NewScryptIdentity(credential)
	if err != nil {
		return nil, fmt.Errorf("vault unwrap identity: %w", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(wrapped), identity)
	if err != nil {
		return nil, fmt.Errorf("vault unwrap: %w", domain.ErrVaultDecryptFailed)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("vault unwrap read: %w", domain.ErrVaultDecryptFailed)
	}

	key, err := age.ParseX25519Identity(string(raw))
	if err != nil {
		return nil, fmt.Errorf("vault unwrap parse: %w", domain.ErrVaultDecryptFailed)
	}
	return key, nil
}

// encryptPayload encrypts vault data to the vault key.
//
// No key derivation runs here, which is the point: this is what every flush calls, and the debounced
// writer fires whenever a connection is edited. Paying 256 MiB of scrypt on each of those was
// affordable only because the old format had no choice.
func encryptPayload(data *domain.VaultData, key *age.X25519Identity) ([]byte, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("vault marshal: %w", err)
	}

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, key.Recipient())
	if err != nil {
		return nil, fmt.Errorf("vault payload init: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("vault payload write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("vault payload close: %w", err)
	}
	return buf.Bytes(), nil
}

// decryptPayload decrypts vault data with the vault key and applies the schema version gates.
func decryptPayload(payload []byte, key *age.X25519Identity) (*domain.VaultData, error) {
	reader, err := age.Decrypt(bytes.NewReader(payload), key)
	if err != nil {
		return nil, fmt.Errorf("vault payload decrypt: %w", domain.ErrVaultDecryptFailed)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("vault payload read: %w", domain.ErrVaultDecryptFailed)
	}
	return decodeVaultData(plaintext)
}
