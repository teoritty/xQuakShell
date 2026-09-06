package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func grantSettingsVault(settings *domain.AppSettings) *revokeTestVault {
	return &revokeTestVault{data: &domain.VaultData{Settings: settings}}
}

// Consent is worthless unless it reaches the vault: an install that recorded nothing would deny the
// plugin on the next launch, with nothing on screen to explain why.
func TestRecordConsentPersistsWhatWasGranted(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	granted := domainplugin.NewPermissionSet([]string{"vault.getSecret:password", "ui.dialogs"})

	if err := svc.RecordConsent(context.Background(), "com.example.sync", granted); err != nil {
		t.Fatalf("RecordConsent err = %v, want nil", err)
	}

	stored, ok := settings.Plugins.GrantFor("com.example.sync")
	if !ok {
		t.Fatal("the consent never reached the vault")
	}
	if len(stored.Granted) != 2 {
		t.Fatalf("stored %v, want both permissions", stored.Granted)
	}
	if stored.GrantedAt.IsZero() {
		t.Error("the stored grant has no timestamp, so an audit cannot say when consent was given")
	}
}

// Re-consenting replaces. A narrower second answer must not leave the wider first one standing and
// doing the deciding.
func TestRecordConsentReplacesAnEarlierGrant(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	ctx := context.Background()

	if err := svc.RecordConsent(ctx, "com.example.sync",
		domainplugin.NewPermissionSet([]string{"vault.getSecret:password", "vault.getSecret:privateKey"})); err != nil {
		t.Fatalf("first RecordConsent err = %v", err)
	}
	if err := svc.RecordConsent(ctx, "com.example.sync",
		domainplugin.NewPermissionSet([]string{"vault.getSecret:password"})); err != nil {
		t.Fatalf("second RecordConsent err = %v", err)
	}

	if svc.IsGranted("com.example.sync", "vault.getSecret:privateKey") {
		t.Error("the withdrawn permission is still granted")
	}
	if !svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Error("the permission that was kept is no longer granted")
	}
}

// The question every enforcement point asks. It has to answer for the exact permission rather than
// for the capability as a whole: a plugin granted the password may not thereby read the private key.
func TestIsGrantedAnswersForTheExactPermission(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	if err := svc.RecordConsent(context.Background(), "com.example.sync",
		domainplugin.NewPermissionSet([]string{"vault.getSecret:password"})); err != nil {
		t.Fatalf("RecordConsent err = %v", err)
	}

	if !svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Error("a recorded permission reports as not granted")
	}
	if svc.IsGranted("com.example.sync", "vault.getSecret:privateKey") {
		t.Error("a permission that was never granted reports as granted")
	}
	if svc.IsGranted("com.example.other", "vault.getSecret:password") {
		t.Error("one plugin's grant answered for another plugin")
	}
}

// A plugin with no recorded consent is granted nothing. This is the direction that has to be safe:
// an unrecorded grant reading as permission would turn a missing record into full access.
func TestIsGrantedRefusesWhenNothingWasRecorded(t *testing.T) {
	svc := NewPluginVaultSettings(grantSettingsVault(&domain.AppSettings{}))

	if svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Fatal("a plugin with no grant was granted a permission")
	}
}

// GrantedTo is what the install path compares a new manifest against, so it must come back as a set
// rather than as raw strings, and an absent grant must be the empty set rather than a nil surprise.
func TestGrantedToReturnsTheRecordedSet(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	granted := domainplugin.NewPermissionSet([]string{"auth.provider", "ui.dialogs"})
	if err := svc.RecordConsent(context.Background(), "com.example.sync", granted); err != nil {
		t.Fatalf("RecordConsent err = %v", err)
	}

	if added, widened := granted.Widens(svc.GrantedTo("com.example.sync")); widened {
		t.Fatalf("the stored set does not cover what was granted, missing %v", added)
	}
	if !svc.GrantedTo("com.example.absent").IsEmpty() {
		t.Error("a plugin with no grant reported permissions")
	}
}

// A vault created before grants existed has no settings section at all. Consent must still be
// recordable rather than failing on an install the user just approved.
func TestRecordConsentCreatesAMissingSettingsSection(t *testing.T) {
	vault := &revokeTestVault{data: &domain.VaultData{}}
	svc := NewPluginVaultSettings(vault)

	if err := svc.RecordConsent(context.Background(), "com.example.sync",
		domainplugin.NewPermissionSet([]string{"ui.dialogs"})); err != nil {
		t.Fatalf("RecordConsent err = %v, want nil", err)
	}

	if !svc.IsGranted("com.example.sync", "ui.dialogs") {
		t.Fatal("consent recorded into a fresh settings section cannot be read back")
	}
}

// Composition can leave this unset, and a permission check that panicked would take down the path
// that decides what a plugin may do. Refusing is the safe answer, and it is the one an unwired
// service must give.
func TestGrantMethodsOnAnUnwiredServiceRefuseRatherThanPanic(t *testing.T) {
	var svc *PluginVaultSettings

	if svc.IsGranted("com.example.sync", "ui.dialogs") {
		t.Error("an unwired service granted a permission")
	}
	if !svc.GrantedTo("com.example.sync").IsEmpty() {
		t.Error("an unwired service reported permissions")
	}
	if err := svc.RecordConsent(context.Background(), "com.example.sync",
		domainplugin.NewPermissionSet([]string{"ui.dialogs"})); err != nil {
		t.Errorf("RecordConsent on an unwired service err = %v, want nil", err)
	}
}

type failingGrantVault struct{ *revokeTestVault }

func (failingGrantVault) GetData() (*domain.VaultData, error) { return nil, domain.ErrVaultLocked }

// A vault that cannot be read must grant nothing. This is the direction that decides whether a
// locked or broken vault fails open: reading the error as anything but "no permissions" would turn
// every read failure into full access for whatever asked at that moment.
func TestAVaultThatCannotBeReadGrantsNothing(t *testing.T) {
	svc := NewPluginVaultSettings(failingGrantVault{&revokeTestVault{data: &domain.VaultData{}}})

	if svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Error("a vault read failure granted a permission")
	}
	if !svc.GrantedTo("com.example.sync").IsEmpty() {
		t.Error("a vault read failure reported permissions")
	}
}

// Uninstall drops the recorded consent along with the old maps, so a plugin reinstalled under the
// same id starts from nothing rather than inheriting a decision about different code.
func TestRevokeAllGrantsRemovesTheRecordedConsent(t *testing.T) {
	settings := &domain.AppSettings{}
	svc := NewPluginVaultSettings(grantSettingsVault(settings))
	ctx := context.Background()
	if err := svc.RecordConsent(ctx, "com.example.sync",
		domainplugin.NewPermissionSet([]string{"vault.getSecret:password"})); err != nil {
		t.Fatalf("RecordConsent err = %v", err)
	}

	if err := svc.RevokeAllGrants(ctx, "com.example.sync"); err != nil {
		t.Fatalf("RevokeAllGrants err = %v", err)
	}

	if svc.IsGranted("com.example.sync", "vault.getSecret:password") {
		t.Fatal("consent survived the uninstall")
	}
}
