package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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

// IsAuthProviderGranted reports whether the recorded grant permits acting as an auth provider.
func (s *PluginVaultSettings) IsAuthProviderGranted(pluginID string) bool {
	return s.IsGranted(pluginID, domainplugin.PermissionAuthProvider)
}

// IsTunnelProviderGranted reports whether the recorded grant permits routing dynamic forwards.
func (s *PluginVaultSettings) IsTunnelProviderGranted(pluginID string) bool {
	return s.IsGranted(pluginID, domainplugin.PermissionTunnelProvider)
}

// IsArbitraryNetworkGranted reports whether the recorded grant permits dialling unnamed hosts.
func (s *PluginVaultSettings) IsArbitraryNetworkGranted(pluginID string) bool {
	return s.IsGranted(pluginID, domainplugin.PermissionArbitraryOutbound)
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

// RevokeAllGrants clears every capability grant recorded for a plugin.
//
// It is called when a plugin is uninstalled. Consent was given to that plugin; once it is gone the
// consent has no subject, and anything installed later under the same id would inherit a decision
// the user made about something else.
func (s *PluginVaultSettings) RevokeAllGrants(ctx context.Context, pluginID string) error {
	if s == nil || s.vault == nil || pluginID == "" {
		return nil
	}
	var revoked []string
	err := s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		// The replication key goes in the same write as the grants. Left behind, it would still
		// open every replica the plugin ever pushed - including the copies on a server the user
		// cannot delete from here - and reinstalling under the same id would pick them straight
		// back up.
		if forgetReplicaKeyLocked(data, pluginID) {
			revoked = append(revoked, "replicaKey")
		}
		if data.Settings == nil {
			return nil
		}
		revoked = append(revoked, data.Settings.Plugins.RevokePluginGrants(pluginID)...)
		return nil
	})
	if err != nil {
		return fmt.Errorf("revoke plugin grants for %s: %w", pluginID, err)
	}
	if len(revoked) > 0 {
		slog.Info("plugin grants revoked on uninstall", "component", "plugin", "plugin", pluginID, "grants", revoked)
	}
	return nil
}
