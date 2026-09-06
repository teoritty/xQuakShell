package usecase

import (
	"context"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func consentTestService(settings *domain.AppSettings) *GitHubPluginService {
	manager := &PluginManager{}
	manager.SetPluginSettings(NewPluginVaultSettings(grantSettingsVault(settings)))
	return &GitHubPluginService{pluginManager: manager}
}

func consentTestManifest() *domainplugin.Manifest {
	return &domainplugin.Manifest{
		ID: "com.example.sync",
		Capabilities: domainplugin.CapabilitySet{
			Vault:  &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			Auth:   &domainplugin.AuthCaps{Provider: true},
			Tunnel: &domainplugin.TunnelCaps{Provider: true},
		},
	}
}

// Installing is the only moment consent is collected, so an install that recorded none would leave
// the plugin with no permissions at all - refused everything, on the strength of a dialog the user
// answered. Nothing else in the flow would report it, because a missing grant reads as "not
// granted" rather than as an error.
func TestInstallRecordsWhatTheUserAgreedTo(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := consentTestService(settings)

	err := svc.recordInstallConsent(context.Background(), consentTestManifest(),
		domainplugin.ConsentFlags{SecretAccess: true, AuthProvider: true})
	if err != nil {
		t.Fatalf("recordInstallConsent err = %v, want nil", err)
	}

	grant, ok := settings.Plugins.GrantFor("com.example.sync")
	if !ok {
		t.Fatal("the install recorded no consent at all")
	}
	for _, want := range []string{
		domainplugin.PermissionSecretField("password"),
		domainplugin.PermissionAuthProvider,
	} {
		if !slices.Contains(grant.Granted, want) {
			t.Errorf("%q was agreed to and is not in the recorded grant: %v", want, grant.Granted)
		}
	}
	if slices.Contains(grant.Granted, domainplugin.PermissionTunnelProvider) {
		t.Errorf("a capability the user declined was recorded: %v", grant.Granted)
	}
}

// An install where every box was left unticked still records a grant - the permissions installing
// alone confers - so there is a baseline for the next version of the plugin to be compared against.
func TestInstallRecordsABaselineWhenEveryBoxIsDeclined(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := consentTestService(settings)

	if err := svc.recordInstallConsent(context.Background(), consentTestManifest(),
		domainplugin.ConsentFlags{}); err != nil {
		t.Fatalf("recordInstallConsent err = %v", err)
	}

	if _, ok := settings.Plugins.GrantFor("com.example.sync"); !ok {
		t.Fatal("declining every box recorded no grant, leaving nothing to compare a later manifest against")
	}
}

// Composition can leave the settings unwired, and a manifest can fail to load. Neither may panic on
// the path that finishes an install.
func TestRecordingInstallConsentIsSafeWithNothingWired(t *testing.T) {
	ctx := context.Background()

	var absent *GitHubPluginService
	if err := absent.recordInstallConsent(ctx, consentTestManifest(), domainplugin.ConsentFlags{}); err != nil {
		t.Errorf("a nil service returned %v", err)
	}
	if err := (&GitHubPluginService{}).recordInstallConsent(ctx, consentTestManifest(), domainplugin.ConsentFlags{}); err != nil {
		t.Errorf("a service with no manager returned %v", err)
	}
	if err := consentTestService(&domain.AppSettings{}).recordInstallConsent(ctx, nil, domainplugin.ConsentFlags{}); err != nil {
		t.Errorf("a nil manifest returned %v", err)
	}
}
