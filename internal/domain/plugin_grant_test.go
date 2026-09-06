package domain_test

import (
	"slices"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

func grantFixture(pluginID string, permissions ...string) domain.PluginGrant {
	return domain.PluginGrant{
		PluginID:  pluginID,
		Granted:   permissions,
		GrantedAt: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC),
	}
}

// A grant records what the user actually agreed to, so it has to come back out unchanged. Consent
// that decoded differently from how it was stored would re-prompt for permissions already given.
func TestRecordGrantStoresWhatWasConsentedTo(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))

	got, ok := settings.GrantFor("com.example.sync")
	if !ok {
		t.Fatal("the grant that was just recorded cannot be found")
	}
	if !slices.Equal(got.Granted, []string{"vault.getSecret:password"}) {
		t.Fatalf("Granted = %v, want the permission that was consented to", got.Granted)
	}
	if got.GrantedAt.IsZero() {
		t.Error("the grant carries no timestamp, so an audit cannot say when consent was given")
	}
}

// Replaces, precisely. Two grants for one plugin would mean either one counts as consent, and
// re-consenting to a narrower set would leave the wider one standing and doing the deciding.
func TestRecordGrantReplacesTheGrantForTheSamePlugin(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password", "vault.getSecret:privateKey"))
	settings.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))

	if n := len(settings.PluginGrants); n != 1 {
		t.Fatalf("there are %d grants for one plugin, want 1: %v", n, settings.PluginGrants)
	}
	got, _ := settings.GrantFor("com.example.sync")
	if slices.Contains(got.Granted, "vault.getSecret:privateKey") {
		t.Fatalf("the replaced grant still carries the withdrawn permission: %v", got.Granted)
	}
}

// Recording one plugin's consent must not touch another's. They are separate decisions about
// separate code.
func TestRecordGrantLeavesOtherPluginsAlone(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.a", "ui.dialogs"))
	settings.RecordGrant(grantFixture("com.example.b", "auth.provider"))

	a, ok := settings.GrantFor("com.example.a")
	if !ok || !slices.Equal(a.Granted, []string{"ui.dialogs"}) {
		t.Fatalf("the first plugin's grant changed: %v", a.Granted)
	}
	if n := len(settings.PluginGrants); n != 2 {
		t.Fatalf("%d grants recorded, want 2", n)
	}
}

// Absence is not an error and must be distinguishable from an empty grant: "never consented" and
// "consented to nothing" lead to different dialogs.
func TestGrantForReportsAnAbsentPlugin(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.sync", "ui.dialogs"))

	if _, ok := settings.GrantFor("com.example.other"); ok {
		t.Fatal("a plugin that was never granted anything reports a grant")
	}
	if _, ok := settings.GrantFor(""); ok {
		t.Fatal("an empty plugin id matched a grant")
	}
}

// Uninstall is a complete remedy (ADR-022 decision 7): the recorded consent goes with the plugin,
// or the next thing installed under that id inherits a decision the user made about something else.
func TestRevokePluginGrantsRemovesTheRecordedGrant(t *testing.T) {
	settings := &domain.PluginSettings{}
	settings.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))
	settings.RecordGrant(grantFixture("com.example.other", "ui.dialogs"))

	revoked := settings.RevokePluginGrants("com.example.sync")

	if _, ok := settings.GrantFor("com.example.sync"); ok {
		t.Fatal("the grant survived the revocation")
	}
	if _, ok := settings.GrantFor("com.example.other"); !ok {
		t.Fatal("revoking one plugin removed another plugin's grant")
	}
	if !slices.Contains(revoked, "permissions") {
		t.Fatalf("revoked = %v, want it to name the permission grant that was removed", revoked)
	}
}

// The caller logs what the user is losing, so a revocation that removed nothing must not claim to
// have removed something.
func TestRevokingAPluginWithNoGrantNamesNothing(t *testing.T) {
	settings := &domain.PluginSettings{}

	if revoked := settings.RevokePluginGrants("com.example.absent"); len(revoked) != 0 {
		t.Fatalf("revoked = %v, want nothing", revoked)
	}
}

// clonePluginSettings lists its fields by hand. A forgotten field produces neither a build error nor
// a failing test - it simply vanishes on every vault read, and consent would stop persisting without
// a single message anywhere.
func TestCloneVaultDataCopiesPluginGrants(t *testing.T) {
	in := domain.NewVaultData()
	in.Settings.Plugins.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))

	out := domain.CloneVaultData(in)

	got, ok := out.Settings.Plugins.GrantFor("com.example.sync")
	if !ok {
		t.Fatal("the clone lost the plugin grant")
	}
	if !slices.Equal(got.Granted, []string{"vault.getSecret:password"}) {
		t.Fatalf("the clone's permissions are %v", got.Granted)
	}
}

// The clone must be deep, and this has to be checked against the stored slice rather than through
// GrantFor, which hands out a copy of its own.
//
// Going through GrantFor is what made an earlier version of this test worthless: clonePluginSettings
// begins with a struct copy, so the slice header comes across even when clonePluginGrants is not
// called at all, and GrantFor's defensive copy hid the sharing underneath. Deleting the clone line
// entirely passed. A shallow clone means an edit to a snapshot rewrites stored consent.
func TestCloneVaultDataDeepCopiesPluginGrants(t *testing.T) {
	in := domain.NewVaultData()
	in.Settings.Plugins.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))

	out := domain.CloneVaultData(in)
	in.Settings.Plugins.PluginGrants[0].Granted[0] = "vault.getSecret:privateKey"

	clone, ok := out.Settings.Plugins.GrantFor("com.example.sync")
	if !ok {
		t.Fatal("the clone lost the plugin grant")
	}
	if clone.Granted[0] != "vault.getSecret:password" {
		t.Fatalf("the clone shares its permission array with the original: %v", clone.Granted)
	}

	in.Settings.Plugins.PluginGrants = append(in.Settings.Plugins.PluginGrants,
		grantFixture("com.example.late", "ui.dialogs"))
	if n := len(out.Settings.Plugins.PluginGrants); n != 1 {
		t.Fatalf("the clone grew to %d grants when the original did", n)
	}
}

// A grant with no plugin id belongs to no plugin, so it must not be storable. Otherwise every
// caller that passed an empty id would share one anonymous grant.
func TestRecordGrantRefusesAGrantWithNoPluginID(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("", "vault.getSecret:password"))

	if n := len(settings.PluginGrants); n != 0 {
		t.Fatalf("%d grants recorded for an empty plugin id: %v", n, settings.PluginGrants)
	}
}

// The vault is a file. A hand-edited or corrupted one can hold a grant with an empty id that
// RecordGrant would have refused, and a lookup by empty id must not match it.
func TestGrantForRefusesAnEmptyIDEvenWhenOneIsStored(t *testing.T) {
	settings := domain.PluginSettings{
		PluginGrants: []domain.PluginGrant{{Granted: []string{"vault.getSecret:password"}}},
	}

	if _, ok := settings.GrantFor(""); ok {
		t.Fatal("an empty plugin id matched a stored grant that has no id either")
	}
}

// A grant taken from settings must not be a window into them either: a caller that edited the slice
// it was handed would rewrite recorded consent without going through RecordGrant.
func TestGrantForHandsOutACopy(t *testing.T) {
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.sync", "vault.getSecret:password"))

	got, _ := settings.GrantFor("com.example.sync")
	got.Granted[0] = "vault.getSecret:privateKey"

	again, _ := settings.GrantFor("com.example.sync")
	if again.Granted[0] != "vault.getSecret:password" {
		t.Fatalf("stored consent was edited through the returned grant: %v", again.Granted)
	}
}

// RecordGrant must not keep the caller's slice either. The caller built it from a manifest and may
// reuse the backing array for the next plugin.
func TestRecordGrantCopiesThePermissionsItIsGiven(t *testing.T) {
	permissions := []string{"vault.getSecret:password"}
	var settings domain.PluginSettings
	settings.RecordGrant(grantFixture("com.example.sync", permissions...))

	permissions[0] = "vault.getSecret:privateKey"

	got, _ := settings.GrantFor("com.example.sync")
	if got.Granted[0] != "vault.getSecret:password" {
		t.Fatalf("the stored grant shares its array with the caller: %v", got.Granted)
	}
}

// These run on settings loaded from a vault that may predate grants entirely, so the nil receiver
// and the nil slice are ordinary states rather than programming errors, and neither may panic on a
// path that decides what a plugin is allowed to do.
func TestGrantMethodsAreSafeOnNilSettings(t *testing.T) {
	var settings *domain.PluginSettings

	if _, ok := settings.GrantFor("com.example.sync"); ok {
		t.Error("nil settings reported a grant")
	}
	settings.RecordGrant(grantFixture("com.example.sync", "ui.dialogs"))
	if revoked := settings.RevokePluginGrants("com.example.sync"); len(revoked) != 0 {
		t.Errorf("nil settings revoked %v", revoked)
	}
}
