package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// The three provider checks each answer for one permission, and each must answer from the recorded
// grant rather than from the boolean map it replaced.
//
// The legacy case is the one that matters. A vault still holding the old boolean - because the
// migration has not run, or failed - must not grant on its own, or the switch to grants would leave
// the old answer in charge and be cosmetic.
func TestTheProviderChecksAnswerFromTheGrant(t *testing.T) {
	const pluginID = "com.example.sync"

	cases := []struct {
		name       string
		permission string
		ask        func(*PluginVaultSettings) bool
		setLegacy  func(*domain.PluginSettings)
	}{
		{
			name:       "auth provider",
			permission: domainplugin.PermissionAuthProvider,
			ask:        func(s *PluginVaultSettings) bool { return s.IsAuthProviderGranted(pluginID) },
			setLegacy: func(p *domain.PluginSettings) {
				p.AuthProviderAccessGranted = map[string]bool{pluginID: true}
			},
		},
		{
			name:       "tunnel provider",
			permission: domainplugin.PermissionTunnelProvider,
			ask:        func(s *PluginVaultSettings) bool { return s.IsTunnelProviderGranted(pluginID) },
			setLegacy: func(p *domain.PluginSettings) {
				p.TunnelProviderAccessGranted = map[string]bool{pluginID: true}
			},
		},
		{
			name:       "arbitrary network",
			permission: domainplugin.PermissionArbitraryOutbound,
			ask:        func(s *PluginVaultSettings) bool { return s.IsArbitraryNetworkGranted(pluginID) },
			setLegacy: func(p *domain.PluginSettings) {
				p.ArbitraryNetworkAccessGranted = map[string]bool{pluginID: true}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &domain.AppSettings{}
			svc := NewPluginVaultSettings(grantSettingsVault(settings))

			if tc.ask(svc) {
				t.Fatal("a plugin with no grant was reported as granted")
			}

			tc.setLegacy(&settings.Plugins)
			if tc.ask(svc) {
				t.Fatal("the boolean map granted on its own, so the grant is not the authority")
			}

			if err := svc.RecordConsent(context.Background(), pluginID,
				domainplugin.NewPermissionSet([]string{tc.permission})); err != nil {
				t.Fatalf("RecordConsent err = %v", err)
			}
			if !tc.ask(svc) {
				t.Fatalf("the recorded permission %q was not honoured", tc.permission)
			}
		})
	}
}

// Each check answers for its own permission only. One provider grant standing in for another would
// hand a plugin a role the user refused, and all three read from the same recorded set - so nothing
// but the permission name keeps them apart.
func TestTheProviderChecksDoNotAnswerForEachOther(t *testing.T) {
	const pluginID = "com.example.sync"
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))

	if err := svc.RecordConsent(context.Background(), pluginID,
		domainplugin.NewPermissionSet([]string{domainplugin.PermissionAuthProvider})); err != nil {
		t.Fatalf("RecordConsent err = %v", err)
	}

	if !svc.IsAuthProviderGranted(pluginID) {
		t.Fatal("the granted role was refused")
	}
	if svc.IsTunnelProviderGranted(pluginID) {
		t.Error("an auth provider grant answered for the tunnel provider role")
	}
	if svc.IsArbitraryNetworkGranted(pluginID) {
		t.Error("an auth provider grant answered for arbitrary network access")
	}
}
