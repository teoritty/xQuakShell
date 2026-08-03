package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
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

// GrantAuthProviderAccess records install-time consent for auth.provider plugins.
func (s *PluginVaultSettings) GrantAuthProviderAccess(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.AuthProviderAccessGranted == nil {
			data.Settings.Plugins.AuthProviderAccessGranted = make(map[string]bool)
		}
		data.Settings.Plugins.AuthProviderAccessGranted[pluginID] = true
		return nil
	})
}

// IsAuthProviderGranted reports whether install-time auth provider consent was recorded.
func (s *PluginVaultSettings) IsAuthProviderGranted(pluginID string) bool {
	if s == nil || s.vault == nil {
		return false
	}
	data, err := s.vault.GetData()
	if err != nil || data.Settings == nil {
		return false
	}
	return data.Settings.Plugins.AuthProviderAccessGranted[pluginID]
}

// GrantTunnelProviderAccess records install-time consent for tunnel.provider plugins.
func (s *PluginVaultSettings) GrantTunnelProviderAccess(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.TunnelProviderAccessGranted == nil {
			data.Settings.Plugins.TunnelProviderAccessGranted = make(map[string]bool)
		}
		data.Settings.Plugins.TunnelProviderAccessGranted[pluginID] = true
		return nil
	})
}

// IsTunnelProviderGranted reports whether install-time tunnel provider consent was recorded.
func (s *PluginVaultSettings) IsTunnelProviderGranted(pluginID string) bool {
	if s == nil || s.vault == nil {
		return false
	}
	data, err := s.vault.GetData()
	if err != nil || data.Settings == nil {
		return false
	}
	return data.Settings.Plugins.TunnelProviderAccessGranted[pluginID]
}

// GrantArbitraryNetworkAccess records install-time consent for allowArbitraryOutbound plugins.
func (s *PluginVaultSettings) GrantArbitraryNetworkAccess(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if data.Settings.Plugins.ArbitraryNetworkAccessGranted == nil {
			data.Settings.Plugins.ArbitraryNetworkAccessGranted = make(map[string]bool)
		}
		data.Settings.Plugins.ArbitraryNetworkAccessGranted[pluginID] = true
		return nil
	})
}

// IsArbitraryNetworkGranted reports whether install-time consent was recorded for pluginID.
func (s *PluginVaultSettings) IsArbitraryNetworkGranted(pluginID string) bool {
	if s == nil || s.vault == nil {
		return false
	}
	data, err := s.vault.GetData()
	if err != nil || data.Settings == nil {
		return false
	}
	return data.Settings.Plugins.ArbitraryNetworkAccessGranted[pluginID]
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
