package usecase

import (
	"context"
	"fmt"

	"ssh-client/internal/domain"
)

// PluginVaultSettings reads and persists plugin secret-access grants from vault settings.
type PluginVaultSettings struct {
	vault domain.VaultRepository
}

// NewPluginVaultSettings creates a vault-backed plugin settings adapter.
func NewPluginVaultSettings(vault domain.VaultRepository) *PluginVaultSettings {
	return &PluginVaultSettings{vault: vault}
}

// PluginSettings implements PluginSettingsReader.
func (s *PluginVaultSettings) PluginSettings() (domain.PluginSettings, error) {
	if s == nil || s.vault == nil {
		return domain.DefaultPluginSettings(), nil
	}
	data, err := s.vault.GetData()
	if err != nil {
		return domain.PluginSettings{}, err
	}
	if data.Settings == nil {
		return domain.DefaultPluginSettings(), nil
	}
	return data.Settings.Plugins, nil
}

// GrantSecretAccess records install-time consent for vault.getSecret.
func (s *PluginVaultSettings) GrantSecretAccess(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.SecretAccessGranted == nil {
			data.Settings.Plugins.SecretAccessGranted = make(map[string]bool)
		}
		data.Settings.Plugins.SecretAccessGranted[pluginID] = true
		return nil
	})
}

// GrantMultiSessionAccess records install-time consent for allowMultiSession plugins.
func (s *PluginVaultSettings) GrantMultiSessionAccess(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.MultiSessionAccessGranted == nil {
			data.Settings.Plugins.MultiSessionAccessGranted = make(map[string]bool)
		}
		data.Settings.Plugins.MultiSessionAccessGranted[pluginID] = true
		return nil
	})
}

// SetPluginEnabled toggles whether a plugin is allowed to run.
func (s *PluginVaultSettings) SetPluginEnabled(ctx context.Context, pluginID string, enabled bool) error {
	if s == nil || s.vault == nil {
		return fmt.Errorf("vault unavailable")
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.Disabled == nil {
			data.Settings.Plugins.Disabled = make(map[string]bool)
		}
		if enabled {
			delete(data.Settings.Plugins.Disabled, pluginID)
		} else {
			data.Settings.Plugins.Disabled[pluginID] = true
		}
		return nil
	})
}
