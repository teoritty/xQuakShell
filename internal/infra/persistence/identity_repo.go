package persistence

import (
	"context"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"ssh-client/internal/domain"
)

// IdentityRepo implements domain.IdentityRepository backed by the vault.
type IdentityRepo struct {
	vault domain.VaultRepository
}

// NewIdentityRepo creates an IdentityRepo backed by the given VaultRepository.
func NewIdentityRepo(v domain.VaultRepository) *IdentityRepo {
	return &IdentityRepo{vault: v}
}

// GetAll returns metadata for every SSH identity in the vault.
func (r *IdentityRepo) GetAll(ctx context.Context) ([]domain.SSHIdentity, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get all identities: %w", err)
	}

	result := make([]domain.SSHIdentity, 0, len(data.Identities))
	for _, id := range data.Identities {
		result = append(result, id)
	}
	return result, nil
}

// GetKeyBlob returns the raw PEM bytes for the identity with the given ID.
func (r *IdentityRepo) GetKeyBlob(ctx context.Context, id string) ([]byte, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get key blob: %w", err)
	}

	blob, ok := data.KeyBlobs[id]
	if !ok {
		return nil, fmt.Errorf("identity %s: %w", id, domain.ErrIdentityNotFound)
	}
	return blob.PEMData, nil
}

// Import stores a new SSH identity in the vault and returns its metadata.
// The PEM bytes are stored as-is (encrypted keys remain encrypted in vault).
func (r *IdentityRepo) Import(ctx context.Context, pemData []byte, comment string) (*domain.SSHIdentity, error) {
	keyType, encrypted := detectKeyType(pemData)

	id := uuid.New().String()
	identity := domain.SSHIdentity{
		ID:        id,
		Comment:   comment,
		KeyType:   keyType,
		Encrypted: encrypted,
	}

	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("import identity get data: %w", err)
	}

	data.Identities[id] = identity
	data.KeyBlobs[id] = domain.IdentityBlob{PEMData: pemData}

	if err := r.vault.SaveData(ctx, data); err != nil {
		return nil, fmt.Errorf("import identity save: %w", err)
	}

	return &identity, nil
}

// Delete removes an identity by ID from the vault.
func (r *IdentityRepo) Delete(ctx context.Context, id string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("delete identity get data: %w", err)
	}

	delete(data.Identities, id)
	delete(data.KeyBlobs, id)

	return r.vault.SaveData(ctx, data)
}

// detectKeyType inspects PEM data to determine key algorithm and encryption status.
func detectKeyType(pemData []byte) (keyType string, encrypted bool) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return "unknown", false
	}

	typeLower := strings.ToLower(block.Type)
	encrypted = strings.Contains(typeLower, "encrypted") ||
		(block.Headers != nil && block.Headers["Proc-Type"] == "4,ENCRYPTED")

	switch {
	case strings.Contains(typeLower, "rsa"):
		keyType = "rsa"
	case strings.Contains(typeLower, "ec"):
		keyType = "ecdsa"
	case strings.Contains(typeLower, "openssh"):
		keyType = "openssh"
	default:
		keyType = "unknown"
	}
	return
}
