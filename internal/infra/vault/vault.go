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

	var data domain.VaultData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("vault unmarshal: %w", err)
	}

	return &data, nil
}
