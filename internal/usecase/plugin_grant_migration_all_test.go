package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func installedWith(id string, caps domainplugin.CapabilitySet) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: id, Capabilities: caps},
	}
}

// Every plugin that is installed needs a recorded grant, not just the ones with elevated
// permissions: without one there is no baseline, and the next version could add an elevated
// permission with nothing to compare it against.
func TestEveryInstalledPluginGetsAGrant(t *testing.T) {
	settings := migrationSettings(map[string]bool{"secret": true})
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	installed := []domainplugin.InstalledPlugin{
		installedWith("com.example.sync", domainplugin.CapabilitySet{
			Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
		}),
		installedWith("com.example.modest", domainplugin.CapabilitySet{
			UI: &domainplugin.UICaps{Dialogs: true},
		}),
	}

	recorded, err := svc.EnsureConsentRecordedForAll(context.Background(), installed)
	if err != nil {
		t.Fatalf("EnsureConsentRecordedForAll err = %v, want nil", err)
	}

	if recorded != 2 {
		t.Fatalf("recorded %d grants, want 2", recorded)
	}
	if !svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Error("the plugin the old maps allowed did not keep its permission")
	}
	if !svc.IsGranted("com.example.modest", "ui.dialogs") {
		t.Error("the modest plugin got no baseline grant")
	}
}

// The migration runs on every unlock, so the second time round there is nothing to do. Counting or
// rewriting grants that already exist would undo a later, narrower re-consent.
func TestASecondPassRecordsNothing(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(migrationSettings(nil)))
	installed := []domainplugin.InstalledPlugin{
		installedWith("com.example.sync", domainplugin.CapabilitySet{
			UI: &domainplugin.UICaps{Dialogs: true},
		}),
	}
	ctx := context.Background()
	if _, err := svc.EnsureConsentRecordedForAll(ctx, installed); err != nil {
		t.Fatalf("first pass err = %v", err)
	}

	recorded, err := svc.EnsureConsentRecordedForAll(ctx, installed)
	if err != nil {
		t.Fatalf("second pass err = %v", err)
	}
	if recorded != 0 {
		t.Fatalf("the second pass recorded %d grants, want none", recorded)
	}
}

// One unusable manifest must not cost the others their grants. This runs once, at unlock, and a
// plugin skipped here has no recorded consent at all until the next one.
func TestOneUnusableManifestDoesNotStopTheRest(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(&domain.AppSettings{}))
	installed := []domainplugin.InstalledPlugin{
		installedWith("", domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}}),
		installedWith("com.example.sync", domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}}),
	}

	recorded, err := svc.EnsureConsentRecordedForAll(context.Background(), installed)
	if err != nil {
		t.Fatalf("EnsureConsentRecordedForAll err = %v", err)
	}

	if recorded != 1 {
		t.Fatalf("recorded %d grants, want 1 - the nameless manifest is skipped, the real one is not", recorded)
	}
	if !svc.IsGranted("com.example.sync", "ui.dialogs") {
		t.Error("a nameless manifest earlier in the list cost a real plugin its grant")
	}
}

type flakyGrantVault struct {
	*revokeTestVault
	failuresLeft int
}

func (v *flakyGrantVault) UpdateData(ctx context.Context, mutate func(*domain.VaultData) error) error {
	if v.failuresLeft > 0 {
		v.failuresLeft--
		return domain.ErrVaultLocked
	}
	return v.revokeTestVault.UpdateData(ctx, mutate)
}

// A write that failed for one plugin must not cost the rest their grants either. This is the same
// rule as for an unusable manifest, but along the path that actually happens in the field - a write
// that failed once - and it is the one a test can otherwise miss, because skipping a nameless
// manifest never produces an error to continue past.
func TestAFailedWriteForOnePluginDoesNotStopTheRest(t *testing.T) {
	vault := &flakyGrantVault{revokeTestVault: &revokeTestVault{data: &domain.VaultData{}}, failuresLeft: 1}
	svc := NewPluginVaultSettings(vault)
	installed := []domainplugin.InstalledPlugin{
		installedWith("com.example.first", domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}}),
		installedWith("com.example.second", domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}}),
	}

	recorded, err := svc.EnsureConsentRecordedForAll(context.Background(), installed)

	if err == nil {
		t.Fatal("the failed write was not reported")
	}
	if recorded != 1 {
		t.Fatalf("recorded = %d, want 1 - the second plugin's grant must survive the first one failing", recorded)
	}
	if !svc.IsGranted("com.example.second", "ui.dialogs") {
		t.Error("the plugin after the failure got no grant")
	}
}

// A vault that refuses writes must be reported rather than reported as done. The caller logs it and
// the next unlock tries again; claiming success would make the retry never happen.
func TestABatchAgainstAnUnwritableVaultReportsTheFailure(t *testing.T) {
	svc := NewPluginVaultSettings(readOnlyGrantVault{&revokeTestVault{data: &domain.VaultData{}}})
	installed := []domainplugin.InstalledPlugin{
		installedWith("com.example.sync", domainplugin.CapabilitySet{UI: &domainplugin.UICaps{Dialogs: true}}),
	}

	recorded, err := svc.EnsureConsentRecordedForAll(context.Background(), installed)

	if err == nil {
		t.Fatal("an unwritable vault was reported as a successful migration")
	}
	if recorded != 0 {
		t.Errorf("recorded = %d, want 0 when nothing could be written", recorded)
	}
}

// Nothing installed is the ordinary state of a fresh vault, not an error.
func TestAnEmptyPluginListIsNotAFailure(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(&domain.AppSettings{}))

	recorded, err := svc.EnsureConsentRecordedForAll(context.Background(), nil)
	if err != nil || recorded != 0 {
		t.Fatalf("recorded = %d, err = %v, want 0 and nil", recorded, err)
	}
}
