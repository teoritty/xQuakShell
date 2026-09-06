package usecase

import (
	"context"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func migrationManifest() *domainplugin.Manifest {
	return &domainplugin.Manifest{
		ID: "com.example.sync",
		Capabilities: domainplugin.CapabilitySet{
			Vault:   &domainplugin.VaultCaps{GetSecret: []string{"password"}, ReadConnectionFields: []string{"host"}},
			Auth:    &domainplugin.AuthCaps{Provider: true},
			Tunnel:  &domainplugin.TunnelCaps{Provider: true},
			Session: &domainplugin.SessionCaps{AllowMultiSession: true, Terminal: true},
			Network: &domainplugin.NetworkCaps{AllowArbitraryOutbound: true},
			Channel: &domainplugin.ChannelCaps{
				Purposes:     []string{domainplugin.PurposeExec},
				ExecCommands: []domainplugin.ExecCommandTemplate{{Argv: []string{"docker", "ps"}}},
			},
		},
	}
}

func migrationSettings(granted map[string]bool) *domain.AppSettings {
	return &domain.AppSettings{Plugins: domain.PluginSettings{
		SecretAccessGranted:           map[string]bool{"com.example.sync": granted["secret"]},
		AuthProviderAccessGranted:     map[string]bool{"com.example.sync": granted["auth"]},
		TunnelProviderAccessGranted:   map[string]bool{"com.example.sync": granted["tunnel"]},
		MultiSessionAccessGranted:     map[string]bool{"com.example.sync": granted["multi"]},
		ArbitraryNetworkAccessGranted: map[string]bool{"com.example.sync": granted["network"]},
	}}
}

// A vault written before grants existed records consent as five booleans. Migration has to carry
// exactly what those booleans already allow - no more, because that would grant something nobody
// agreed to, and no less, because that would break a plugin the user installed deliberately.
func TestMigrationCarriesTheOldBooleansForward(t *testing.T) {
	settings := migrationSettings(map[string]bool{"secret": true, "auth": true})
	svc := NewPluginVaultSettings(grantSettingsVault(settings))

	migrated, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest())
	if err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v, want nil", err)
	}
	if !migrated {
		t.Fatal("a plugin with no recorded grant was not migrated")
	}

	granted := svc.GrantedTo("com.example.sync")
	for _, want := range []string{"vault.getSecret:password", "auth.provider"} {
		if !granted.Has(want) {
			t.Errorf("%q was allowed by the old maps and is not in the migrated grant", want)
		}
	}
	for _, unwanted := range []string{"tunnel.provider", "session.allowMultiSession", "network.allowArbitraryOutbound"} {
		if granted.Has(unwanted) {
			t.Errorf("%q was refused by the old maps and appears in the migrated grant", unwanted)
		}
	}
}

// The permissions installing confers are not in the old maps at all, and they have to survive the
// migration or a plugin loses what nobody ever asked the user about.
func TestMigrationKeepsThePermissionsInstallingConfers(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(migrationSettings(nil)))

	if _, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest()); err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v", err)
	}

	granted := svc.GrantedTo("com.example.sync")
	for _, want := range []string{"vault.readConnectionFields:host", "session.terminal", "channel.purposes:exec"} {
		if !granted.Has(want) {
			t.Errorf("%q is conferred by installing and did not survive the migration", want)
		}
	}
}

// The mirror of the test above, and the one that carries the weight: with every old boolean unset,
// no elevated permission may appear. Asserting only that the granted ones survive would pass just as
// well if migration granted everything, which is the mistake that matters here.
func TestMigrationGrantsNoElevatedPermissionTheOldMapsRefused(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(migrationSettings(nil)))

	if _, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest()); err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v", err)
	}

	granted := svc.GrantedTo("com.example.sync")
	for _, unwanted := range []string{
		"vault.getSecret:password",
		"auth.provider",
		"tunnel.provider",
		"session.allowMultiSession",
		"network.allowArbitraryOutbound",
	} {
		if granted.Has(unwanted) {
			t.Errorf("%q was refused by every old map and appears in the migrated grant", unwanted)
		}
	}
}

// Migration runs at startup, where the vault can be locked or unreadable. It must report the failure
// rather than record a grant built from settings it could not read - an empty read would look like
// "the user granted nothing" and quietly narrow what a plugin is allowed to do.
func TestMigrationReportsAVaultItCannotRead(t *testing.T) {
	svc := NewPluginVaultSettings(failingGrantVault{&revokeTestVault{data: &domain.VaultData{}}})

	migrated, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest())

	if err == nil {
		t.Fatal("migration against an unreadable vault reported success")
	}
	if migrated {
		t.Error("migration claimed to have written a grant it could not build")
	}
}

type readOnlyGrantVault struct{ *revokeTestVault }

func (readOnlyGrantVault) UpdateData(context.Context, func(*domain.VaultData) error) error {
	return domain.ErrVaultLocked
}

// A write that failed must be reported as a failure. Returning "migrated" for a grant that never
// reached the vault would make the caller stop retrying, and the plugin would run on the next start
// with no recorded consent at all.
func TestMigrationReportsAGrantItCouldNotWrite(t *testing.T) {
	svc := NewPluginVaultSettings(readOnlyGrantVault{&revokeTestVault{data: &domain.VaultData{}}})

	migrated, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest())

	if err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if migrated {
		t.Error("migration claimed to have written a grant the vault refused")
	}
}

// Exec consent has no old map: it is checked when the plugin is installed and never stored. So for
// a plugin that is installed, the installation is the record - it could not have got there without
// the user agreeing. A plugin with no exec templates gains nothing from this, because there is
// nothing to gain.
func TestMigrationTreatsBeingInstalledAsExecConsent(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(migrationSettings(nil)))

	if _, err := svc.EnsureConsentRecorded(context.Background(), migrationManifest()); err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v", err)
	}

	if !svc.IsGranted("com.example.sync", "channel.execCommands:docker ps") {
		t.Fatal("an installed plugin's exec template was not carried into its grant")
	}
}

// Migration runs on every start. The second run must find the grant already there and leave it
// alone: recomputing it from the old maps would undo a later, narrower re-consent.
func TestMigrationLeavesAnExistingGrantAlone(t *testing.T) {
	settings := migrationSettings(map[string]bool{"secret": true})
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	ctx := context.Background()
	if err := svc.RecordConsent(ctx, "com.example.sync",
		domainplugin.NewPermissionSet([]string{"ui.dialogs"})); err != nil {
		t.Fatalf("RecordConsent err = %v", err)
	}

	migrated, err := svc.EnsureConsentRecorded(ctx, migrationManifest())
	if err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v", err)
	}

	if migrated {
		t.Error("migration overwrote a grant that was already recorded")
	}
	if got := svc.GrantedTo("com.example.sync").Tokens(); !slices.Equal(got, []string{"ui.dialogs"}) {
		t.Fatalf("the existing grant was rewritten to %v", got)
	}
}

// A plugin that asks for nothing elevated still gets a grant recorded. Without one there is no
// baseline, and the next version could add an elevated permission with nothing to compare against.
func TestMigrationRecordsABaselineForAModestPlugin(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(&domain.AppSettings{}))
	modest := &domainplugin.Manifest{
		ID:           "com.example.modest",
		Capabilities: domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}},
	}

	migrated, err := svc.EnsureConsentRecorded(context.Background(), modest)
	if err != nil {
		t.Fatalf("EnsureConsentRecorded err = %v", err)
	}

	if !migrated {
		t.Fatal("a plugin with no elevated permissions was not given a baseline grant")
	}
	if !svc.IsGranted("com.example.modest", "ui.dialogs") {
		t.Fatal("the baseline grant does not cover what the manifest asks for")
	}
}

// These run during startup, where a half-built composition and a manifest that failed to load are
// both ordinary. Neither may panic on the path that decides what plugins are allowed to do.
func TestMigrationIsSafeWithNothingToWorkFrom(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(&domain.AppSettings{}))

	if migrated, err := svc.EnsureConsentRecorded(context.Background(), nil); migrated || err != nil {
		t.Errorf("a nil manifest migrated = %v, err = %v", migrated, err)
	}
	if migrated, err := svc.EnsureConsentRecorded(context.Background(),
		&domainplugin.Manifest{}); migrated || err != nil {
		t.Errorf("a manifest with no id migrated = %v, err = %v", migrated, err)
	}

	var unwired *PluginVaultSettings
	if migrated, err := unwired.EnsureConsentRecorded(context.Background(), migrationManifest()); migrated || err != nil {
		t.Errorf("an unwired service migrated = %v, err = %v", migrated, err)
	}
}
