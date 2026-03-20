package persistence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"ssh-client/internal/domain"
)

// PasswordRepo implements domain.PasswordRepository backed by the vault.
type PasswordRepo struct {
	vault domain.VaultRepository
}

// NewPasswordRepo creates a PasswordRepo backed by the given VaultRepository.
func NewPasswordRepo(v domain.VaultRepository) *PasswordRepo {
	return &PasswordRepo{vault: v}
}

// Import stores a new password in the vault and returns the generated ID.
func (r *PasswordRepo) Import(ctx context.Context, password []byte, label string) (string, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return "", fmt.Errorf("import password get data: %w", err)
	}

	id := uuid.New().String()
	data.Passwords[id] = domain.PasswordBlob{
		Value: append([]byte(nil), password...),
		Label: label,
	}

	if err := r.vault.SaveData(ctx, data); err != nil {
		return "", fmt.Errorf("import password save: %w", err)
	}

	return id, nil
}

// Get retrieves the raw password bytes for the given ID.
func (r *PasswordRepo) Get(_ context.Context, id string) ([]byte, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get password: %w", err)
	}

	blob, ok := data.Passwords[id]
	if !ok {
		return nil, fmt.Errorf("password %s: %w", id, domain.ErrPasswordNotFound)
	}
	return blob.Value, nil
}

// Delete removes a password by ID from the vault.
func (r *PasswordRepo) Delete(ctx context.Context, id string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("delete password get data: %w", err)
	}

	if _, ok := data.Passwords[id]; !ok {
		return fmt.Errorf("password %s: %w", id, domain.ErrPasswordNotFound)
	}

	delete(data.Passwords, id)
	return r.vault.SaveData(ctx, data)
}

// List returns metadata (ID + label) for all stored passwords without exposing values.
func (r *PasswordRepo) List(_ context.Context) ([]domain.PasswordBlob, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("list passwords: %w", err)
	}

	result := make([]domain.PasswordBlob, 0, len(data.Passwords))
	for id, blob := range data.Passwords {
		result = append(result, domain.PasswordBlob{
			Label: blob.Label,
			Value: []byte(id),
		})
	}
	return result, nil
}
