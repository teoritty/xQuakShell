package usecase

import (
	"context"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// EnsureConsentRecorded gives a plugin a recorded grant if it has none, derived from the boolean
// maps a vault written before ADR-022 used. It reports whether it wrote one.
//
// The migration is deliberately behaviour-preserving. An old boolean already opens every field the
// manifest lists, so carrying it forward through GrantedPermissions grants exactly what the plugin
// can reach today - no more, which would hand it something nobody agreed to, and no less, which
// would break a plugin the user installed on purpose. It does not repair a widening that already
// happened before the grant existed; nothing here can, and the value is that the next one is caught.
//
// It runs on every start and is idempotent by design: an existing grant is left alone, or a later
// and narrower re-consent would be undone by the maps it replaced.
func (s *PluginVaultSettings) EnsureConsentRecorded(
	ctx context.Context,
	manifest *domainplugin.Manifest,
) (bool, error) {
	if s == nil || s.vault == nil || manifest == nil || manifest.ID == "" {
		return false, nil
	}
	if _, recorded := s.grantFor(manifest.ID); recorded {
		return false, nil
	}
	settings, err := s.PluginSettings()
	if err != nil {
		return false, err
	}
	granted := domainplugin.GrantedPermissions(manifest, legacyConsentFor(settings, manifest.ID))
	if err := s.RecordConsent(ctx, manifest.ID, granted); err != nil {
		return false, err
	}
	return true, nil
}

// grantFor reports whether a grant is recorded, without going through GrantedTo - which cannot tell
// "no grant" from "a grant that permits nothing", and migration needs exactly that distinction.
func (s *PluginVaultSettings) grantFor(pluginID string) (domain.PluginGrant, bool) {
	data, err := s.vault.GetData()
	if err != nil || data == nil || data.Settings == nil {
		return domain.PluginGrant{}, false
	}
	return data.Settings.Plugins.GrantFor(pluginID)
}

// legacyConsentFor reads the pre-grant boolean maps for one plugin.
//
// ExecChannel has no map to read: exec consent is checked when a plugin is installed and never
// stored, so for a plugin that IS installed the installation is the record - it could not have got
// there without the user agreeing. A plugin with no exec templates gains nothing from this, because
// GrantedPermissions can only keep what the manifest already asked for.
func legacyConsentFor(settings domain.PluginSettings, pluginID string) domainplugin.ConsentFlags {
	return domainplugin.ConsentFlags{
		SecretAccess:     settings.SecretAccessGranted[pluginID],
		AuthProvider:     settings.AuthProviderAccessGranted[pluginID],
		TunnelProvider:   settings.TunnelProviderAccessGranted[pluginID],
		MultiSession:     settings.MultiSessionAccessGranted[pluginID],
		ArbitraryNetwork: settings.ArbitraryNetworkAccessGranted[pluginID],
		ExecChannel:      true,
	}
}
