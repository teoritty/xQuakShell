package persistence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"ssh-client/internal/domain"
)

// VPNProfileRepo implements domain.VPNProfileRepository backed by the vault.
type VPNProfileRepo struct {
	vault domain.VaultRepository
}

// NewVPNProfileRepo creates a VPNProfileRepo backed by the given VaultRepository.
func NewVPNProfileRepo(v domain.VaultRepository) *VPNProfileRepo {
	return &VPNProfileRepo{vault: v}
}

// Save creates or updates a VPN profile in the vault.
func (r *VPNProfileRepo) Save(ctx context.Context, profile *domain.VPNProfile) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("save vpn profile get data: %w", err)
	}

	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	data.VPNProfiles[profile.ID] = *profile

	return r.vault.SaveData(ctx, data)
}

// Get retrieves a VPN profile by ID.
func (r *VPNProfileRepo) Get(_ context.Context, id string) (*domain.VPNProfile, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get vpn profile: %w", err)
	}

	p, ok := data.VPNProfiles[id]
	if !ok {
		return nil, fmt.Errorf("vpn profile %s: %w", id, domain.ErrVPNProfileNotFound)
	}
	return &p, nil
}

// Delete removes a VPN profile by ID from the vault.
func (r *VPNProfileRepo) Delete(ctx context.Context, id string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("delete vpn profile get data: %w", err)
	}

	if _, ok := data.VPNProfiles[id]; !ok {
		return fmt.Errorf("vpn profile %s: %w", id, domain.ErrVPNProfileNotFound)
	}

	delete(data.VPNProfiles, id)
	return r.vault.SaveData(ctx, data)
}

// GetAll returns every VPN profile stored in the vault.
func (r *VPNProfileRepo) GetAll(_ context.Context) ([]domain.VPNProfile, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get all vpn profiles: %w", err)
	}

	result := make([]domain.VPNProfile, 0, len(data.VPNProfiles))
	for _, p := range data.VPNProfiles {
		result = append(result, p)
	}
	return result, nil
}
