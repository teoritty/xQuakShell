package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
)

type revokeTestVault struct {
	data *domain.VaultData
}

func (v *revokeTestVault) Exists() bool                                       { return true }
func (v *revokeTestVault) Create(context.Context, string) error               { return nil }
func (v *revokeTestVault) Unlock(context.Context, string) error               { return nil }
func (v *revokeTestVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (v *revokeTestVault) Lock()                                              {}
func (v *revokeTestVault) IsUnlocked() bool                                   { return true }
func (v *revokeTestVault) GetData() (*domain.VaultData, error)                { return v.data, nil }
func (v *revokeTestVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(v.data)
}

// The domain function is only worth anything if the uninstall path actually persists its effect.
// RevokeAllGrants is what carries it into the vault, and it used not to exist at all.
func TestRevokeAllGrantsPersistsTheRevocation(t *testing.T) {
	const pluginID = "com.example.tool"
	settings := domain.AppSettings{
		Plugins: domain.PluginSettings{
			SecretAccessGranted:       map[string]bool{pluginID: true, "com.example.other": true},
			AuthProviderAccessGranted: map[string]bool{pluginID: true},
		},
	}
	vault := &revokeTestVault{data: &domain.VaultData{Settings: &settings}}
	svc := NewPluginVaultSettings(vault)

	if err := svc.RevokeAllGrants(context.Background(), pluginID); err != nil {
		t.Fatalf("RevokeAllGrants err = %v, want nil", err)
	}

	if settings.Plugins.SecretAccessGranted[pluginID] {
		t.Error("the uninstalled plugin's secret grant survived in the vault")
	}
	if settings.Plugins.AuthProviderAccessGranted[pluginID] {
		t.Error("the uninstalled plugin's auth provider grant survived in the vault")
	}
	if !settings.Plugins.SecretAccessGranted["com.example.other"] {
		t.Error("another plugin's grant was revoked")
	}
}

// A vault with no settings section at all must not panic, and must not be a reason for uninstall
// to report failure.
func TestRevokeAllGrantsOnAVaultWithNoSettings(t *testing.T) {
	vault := &revokeTestVault{data: &domain.VaultData{}}
	svc := NewPluginVaultSettings(vault)

	if err := svc.RevokeAllGrants(context.Background(), "com.example.tool"); err != nil {
		t.Fatalf("RevokeAllGrants err = %v, want nil", err)
	}
}

func TestRevokeAllGrantsIgnoresAnEmptyPluginID(t *testing.T) {
	const other = "com.example.other"
	settings := domain.AppSettings{
		Plugins: domain.PluginSettings{SecretAccessGranted: map[string]bool{other: true}},
	}
	vault := &revokeTestVault{data: &domain.VaultData{Settings: &settings}}
	svc := NewPluginVaultSettings(vault)

	if err := svc.RevokeAllGrants(context.Background(), ""); err != nil {
		t.Fatalf("RevokeAllGrants err = %v, want nil", err)
	}
	if !settings.Plugins.SecretAccessGranted[other] {
		t.Error("an empty plugin id revoked someone else's grant")
	}
}
