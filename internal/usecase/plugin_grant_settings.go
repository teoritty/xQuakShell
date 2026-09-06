package usecase

import (
	"context"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// RecordConsent stores the permissions the user agreed to for a plugin (ADR-022).
//
// It replaces whatever was recorded before rather than adding to it: re-consenting to a narrower set
// must not leave a wider one standing, and a plugin that gave a permission up should stop being
// granted it.
func (s *PluginVaultSettings) RecordConsent(
	ctx context.Context,
	pluginID string,
	granted domainplugin.PermissionSet,
) error {
	if s == nil || s.vault == nil {
		return nil
	}
	return s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		data.Settings.Plugins.RecordGrant(domain.PluginGrant{
			PluginID: pluginID,
			Granted:  granted.Tokens(),
			// UTC because this is read back by an audit that has to line up with everything else it
			// records, not by anyone reading a local clock.
			GrantedAt: time.Now().UTC(),
		})
		return nil
	})
}

// GrantedTo returns the permissions recorded for a plugin, empty when none were.
//
// Empty is the safe reading of every failure here - an unwired service, a locked vault, a plugin
// nobody consented to - because the caller turns this into a permission decision, and a missing
// record must never read as permission.
func (s *PluginVaultSettings) GrantedTo(pluginID string) domainplugin.PermissionSet {
	if s == nil || s.vault == nil {
		return domainplugin.PermissionSet{}
	}
	data, err := s.vault.GetData()
	if err != nil || data == nil || data.Settings == nil {
		return domainplugin.PermissionSet{}
	}
	// A plugin nobody consented to comes back as the zero grant, whose permission list is empty, so
	// absence needs no branch of its own here - and a branch no test can falsify is one that will
	// quietly rot.
	grant, _ := data.Settings.Plugins.GrantFor(pluginID)
	return domainplugin.NewPermissionSet(grant.Granted)
}

// IsGranted reports whether one permission is covered by the consent recorded for a plugin.
//
// It answers for the exact permission rather than for the capability around it: a plugin granted a
// connection password has not thereby been granted the private key, even though both arrive through
// vault.getSecret.
func (s *PluginVaultSettings) IsGranted(pluginID, permission string) bool {
	return s.GrantedTo(pluginID).Has(permission)
}
