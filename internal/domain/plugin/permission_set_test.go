package plugin

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// These tests live inside the package rather than beside it so that the classification tests can
// read permissionExclusions directly. The alternative was a second copy of that list in the test,
// and two copies of one decision are two decisions waiting to disagree.

// fullyPopulatedCapabilities names every capability the manifest can carry, with every field set to
// something non-zero. It is the fixture the two completeness tests rest on: one checks that it
// really is complete, the other that the permission mapper accounts for everything in it.
func fullyPopulatedCapabilities() CapabilitySet {
	return CapabilitySet{
		Network: &NetworkCaps{
			Outbound:               []string{"tcp:example.com:443"},
			AllowArbitraryOutbound: true,
			AllowPrivateNetworks:   true,
		},
		FS: &FSCaps{
			Read:  []string{"${pluginData}/in"},
			Write: []string{"${pluginData}/out"},
		},
		Events: &EventCaps{
			Subscribe: []string{"core.session"},
			Publish:   []string{"com.example.sync/done"},
		},
		Vault: &VaultCaps{
			ReadConnectionFields: []string{"host"},
			GetSecret:            []string{"password"},
		},
		Session: &SessionCaps{
			ConnectProtocols:       []string{"ssh"},
			Terminal:               true,
			Embed:                  true,
			RemoteFS:               true,
			AllowMultiSession:      true,
			MaxTunnelBandwidthKbps: 1024,
		},
		Auth:   &AuthCaps{Provider: true, Methods: []string{"publickey"}},
		Tunnel: &TunnelCaps{Provider: true, MaxConcurrentChannels: 4},
		Channel: &ChannelCaps{
			Purposes:          []string{PurposeExec},
			MaxConcurrent:     2,
			MaxThroughputKbps: 512,
			ExecCommands: []ExecCommandTemplate{
				{Argv: []string{"docker", "ps"}, Params: map[string]string{"host": "string"}},
			},
		},
		Discovery: &DiscoveryCaps{ParentProtocols: []string{"ssh"}},
		UI: &UICaps{
			Surfaces:    []string{"terminal"},
			Dialogs:     true,
			NodeDetails: true,
			MaxSurfaces: 3,
		},
		I18n: &I18nCaps{Locales: []string{"en"}},
	}
}

// capabilityFieldNames walks the manifest capability structs the way the mapper has to, and returns
// "<capability>.<field>" for every leaf, taken from the JSON tags so the names match the wire
// contract rather than the Go spelling.
func capabilityFieldNames(t *testing.T) []string {
	t.Helper()
	var names []string
	set := reflect.TypeOf(CapabilitySet{})
	for i := range set.NumField() {
		capField := set.Field(i)
		capName := jsonFieldName(capField)
		caps := capField.Type
		if caps.Kind() == reflect.Ptr {
			caps = caps.Elem()
		}
		if caps.Kind() != reflect.Struct {
			t.Fatalf("capability %q is not a struct", capName)
		}
		for j := range caps.NumField() {
			names = append(names, capName+"."+jsonFieldName(caps.Field(j)))
		}
	}
	return names
}

func jsonFieldName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		return f.Name
	}
	return name
}

// The fixture is worthless if it is not complete: a field left at its zero value would make the
// classification test below pass by accident, because the mapper would emit nothing for it and
// nobody would notice.
func TestCapabilityFixtureLeavesNoFieldUnset(t *testing.T) {
	set := reflect.ValueOf(fullyPopulatedCapabilities())
	setType := set.Type()

	for i := range set.NumField() {
		capValue := set.Field(i)
		capName := jsonFieldName(setType.Field(i))
		if capValue.IsNil() {
			t.Errorf("capability %q is not populated in the fixture", capName)
			continue
		}
		inner := capValue.Elem()
		for j := range inner.NumField() {
			if inner.Field(j).IsZero() {
				t.Errorf("%s.%s is left at its zero value in the fixture",
					capName, jsonFieldName(inner.Type().Field(j)))
			}
		}
	}
}

// Every capability field must be accounted for: it either produces a permission token or it is on
// the permissionExclusions list with a recorded reason. A field that is neither is one a plugin can
// widen without the user being asked, which is the whole failure this type exists to prevent.
func TestEveryCapabilityFieldIsEitherAPermissionOrExcluded(t *testing.T) {
	m := &Manifest{ID: "com.example.sync", Capabilities: fullyPopulatedCapabilities()}
	tokens := PermissionSetFromManifest(m).Tokens()

	for _, field := range capabilityFieldNames(t) {
		if slices.Contains(permissionExclusions, field) {
			continue
		}
		if !slices.ContainsFunc(tokens, func(tok string) bool {
			return tok == field || strings.HasPrefix(tok, field+":")
		}) {
			t.Errorf("capability field %q produces no permission token and is not excluded; "+
				"either map it or record in permissionExclusions why it grants nothing", field)
		}
	}
}

// A stale exclusion is its own hazard: a field that was renamed away leaves an entry that looks like
// a considered decision and covers nothing.
func TestEveryExclusionNamesARealCapabilityField(t *testing.T) {
	fields := capabilityFieldNames(t)
	for _, excluded := range permissionExclusions {
		if !slices.Contains(fields, excluded) {
			t.Errorf("permissionExclusions lists %q, which is not a capability field any more", excluded)
		}
	}
}

// The point of the whole type: a plugin that asks for more must be caught, and the user must be told
// what was added rather than that "permissions changed".
func TestWidensNamesWhatWasAdded(t *testing.T) {
	granted := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password"}}},
	})
	updated := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password", "privateKey"}}},
	})

	added, widened := updated.Widens(granted)
	if !widened {
		t.Fatal("asking for privateKey on top of password was not reported as a widening")
	}
	if len(added) != 1 || added[0] != "vault.getSecret:privateKey" {
		t.Fatalf("added = %v, want exactly [vault.getSecret:privateKey]", added)
	}
}

// An update that drops a permission is not a widening. Prompting for it would train people to click
// through the dialog that matters.
func TestGivingUpAPermissionIsNotAWidening(t *testing.T) {
	granted := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password", "privateKey"}}},
	})
	updated := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password"}}},
	})

	if added, widened := updated.Widens(granted); widened {
		t.Fatalf("dropping privateKey was reported as a widening, adding %v", added)
	}
}

// An identical manifest must pass silently. This is the ordinary case - most updates change code,
// not permissions - and it is the case that decides whether the dialog stays meaningful.
func TestAnUnchangedManifestDoesNotWiden(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: fullyPopulatedCapabilities(),
	})

	if added, widened := set.Widens(set); widened {
		t.Fatalf("an unchanged manifest widened, adding %v", added)
	}
}

// Order in the manifest is the plugin author's business and says nothing about permissions. If it
// leaked into the comparison, reordering a list would raise a dialog that names nothing.
func TestPermissionSetIgnoresTheOrderFieldsWereWrittenIn(t *testing.T) {
	first := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Network: &NetworkCaps{
			Outbound: []string{"tcp:a.example:1", "tcp:b.example:2"},
		}},
	})
	second := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Network: &NetworkCaps{
			Outbound: []string{"tcp:b.example:2", "tcp:a.example:1"},
		}},
	})

	if added, widened := second.Widens(first); widened {
		t.Fatalf("reordering the outbound list widened, adding %v", added)
	}
	if !slices.Equal(first.Tokens(), second.Tokens()) {
		t.Fatalf("tokens differ by input order:\n %v\n %v", first.Tokens(), second.Tokens())
	}
}

// A capability block can be present with its flags off. Those flags ask for nothing and must
// contribute nothing: a mapper that emitted a token regardless of the value would record every
// manifest as requesting every flag, and the first update to actually set one would then look like
// no change at all.
func TestAFlagThatIsOffGrantsNothing(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{
			Network: &NetworkCaps{Outbound: []string{"tcp:a.example:1"}},
			Session: &SessionCaps{ConnectProtocols: []string{"ssh"}},
			Auth:    &AuthCaps{},
			Tunnel:  &TunnelCaps{},
			UI:      &UICaps{},
		},
	})

	want := []string{"network.outbound:tcp:a.example:1", "session.connectProtocols:ssh"}
	if got := set.Tokens(); !slices.Equal(got, want) {
		t.Fatalf("tokens = %v, want only the value-bearing fields %v", got, want)
	}
}

// A duplicate entry is a manifest typo, not a second permission. Counting it twice would make a set
// compare unequal to itself after a harmless edit.
func TestPermissionSetDropsDuplicates(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password", "password"}}},
	})

	if got := set.Tokens(); len(got) != 1 {
		t.Fatalf("tokens = %v, want one entry", got)
	}
}

// Exec commands are the highest-risk thing a manifest can carry - a new argv template is a new
// command the host will run on the plugin's behalf - so any change to one must reach the user.
func TestAddingAnExecCommandIsAWidening(t *testing.T) {
	granted := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{
			Purposes:     []string{PurposeExec},
			ExecCommands: []ExecCommandTemplate{{Argv: []string{"docker", "ps"}}},
		}},
	})
	updated := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{
			Purposes: []string{PurposeExec},
			ExecCommands: []ExecCommandTemplate{
				{Argv: []string{"docker", "ps"}},
				{Argv: []string{"docker", "exec"}},
			},
		}},
	})

	added, widened := updated.Widens(granted)
	if !widened {
		t.Fatal("a new exec template was not reported as a widening")
	}
	if len(added) != 1 || !strings.Contains(added[0], "docker exec") {
		t.Fatalf("added = %v, want the new template named", added)
	}
}

// Substitution parameters decide what a template can be made to run, so changing them changes the
// permission even when the argv is untouched.
func TestChangingExecParametersIsAWidening(t *testing.T) {
	granted := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{
			ExecCommands: []ExecCommandTemplate{{Argv: []string{"docker", "ps"}}},
		}},
	})
	updated := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{
			ExecCommands: []ExecCommandTemplate{
				{Argv: []string{"docker", "ps"}, Params: map[string]string{"host": "string"}},
			},
		}},
	})

	if _, widened := updated.Widens(granted); !widened {
		t.Fatal("adding a substitution parameter to an exec template was not reported as a widening")
	}
}

// Raising a numeric ceiling is deliberately not a widening (see permissionExclusions). The decision
// is pinned here so reversing it has to be done on purpose rather than by editing the mapper.
func TestRaisingANumericLimitIsNotAWidening(t *testing.T) {
	low := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{MaxThroughputKbps: 128}},
	})
	high := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Channel: &ChannelCaps{MaxThroughputKbps: 100000}},
	})

	if added, widened := high.Widens(low); widened {
		t.Fatalf("raising a throughput ceiling widened, adding %v", added)
	}
}

// A plugin that asks for nothing must grant nothing. The empty set is what an install with no
// capabilities records, and it has to compare cleanly against everything else.
func TestAManifestWithNoCapabilitiesGrantsNothing(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{ID: "com.example.quiet"})

	if got := set.Tokens(); len(got) != 0 {
		t.Fatalf("tokens = %v, want none", got)
	}
	if !set.IsEmpty() {
		t.Fatal("a manifest with no capabilities produced a non-empty set")
	}
	if added, widened := set.Widens(set); widened {
		t.Fatalf("the empty set widened against itself, adding %v", added)
	}
}

// A nil manifest reaches this from a caller that failed to load one. Returning the empty set means
// such a caller grants nothing, where a panic would take down a path that handles credentials.
func TestANilManifestGrantsNothing(t *testing.T) {
	if !PermissionSetFromManifest(nil).IsEmpty() {
		t.Fatal("a nil manifest produced permissions")
	}
}

// A first install has nothing granted yet, so everything the manifest asks for is new. This is the
// path that renders the install consent screen.
func TestEverythingIsAWideningAgainstAnEmptyGrant(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: fullyPopulatedCapabilities(),
	})

	added, widened := set.Widens(PermissionSet{})
	if !widened {
		t.Fatal("a fresh install did not report its permissions as new")
	}
	if len(added) != len(set.Tokens()) {
		t.Fatalf("added %d permissions, want all %d", len(added), len(set.Tokens()))
	}
}

// The set is stored in the vault, so it has to survive the trip. A set that decoded differently
// would make every plugin re-prompt after a restart.
func TestPermissionSetSurvivesAJSONRoundTrip(t *testing.T) {
	original := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: fullyPopulatedCapabilities(),
	})

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded PermissionSet
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !slices.Equal(decoded.Tokens(), original.Tokens()) {
		t.Fatalf("round trip changed the set:\n before %v\n after  %v", original.Tokens(), decoded.Tokens())
	}
	if added, widened := decoded.Widens(original); widened {
		t.Fatalf("the decoded set widened against the original, adding %v", added)
	}
}

// A set that grants nothing must store as an empty list, not null. "No record" and "recorded, and
// it is empty" are different states of a grant and must not look alike on disk.
func TestAnEmptyPermissionSetMarshalsAsAnEmptyList(t *testing.T) {
	raw, err := json.Marshal(PermissionSet{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "[]" {
		t.Fatalf("marshalled as %s, want []", raw)
	}
}

// A vault edited by hand can hold an unsorted or duplicated list. Normalizing on the way in keeps
// such a set comparing equal to the one the manifest produces, instead of re-prompting for
// permissions the user already gave.
func TestDecodingNormalizesAnUntidyList(t *testing.T) {
	var decoded PermissionSet
	if err := json.Unmarshal([]byte(`["ui.dialogs","auth.provider","ui.dialogs"]`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{"auth.provider", "ui.dialogs"}
	if got := decoded.Tokens(); !slices.Equal(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
}

// A malformed grant has to surface as an error rather than decode to an empty set. An empty set
// grants nothing and is indistinguishable from a plugin that asked for nothing, so swallowing the
// error would silently drop permissions the user did give - and then re-prompt for them as new.
func TestDecodingRejectsAMalformedGrant(t *testing.T) {
	var decoded PermissionSet
	if err := json.Unmarshal([]byte(`{"not":"a list"}`), &decoded); err == nil {
		t.Fatalf("a malformed permission set decoded without error, as %v", decoded.Tokens())
	}
}

// Has is what every enforcement point calls, so it answers for the exact permission and hands out
// nothing: asking whether one token is present must not require a copy of the whole set.
func TestHasAnswersForOnePermission(t *testing.T) {
	set := NewPermissionSet([]string{"vault.getSecret:password", "ui.dialogs"})

	if !set.Has("vault.getSecret:password") {
		t.Error("a permission in the set reports as absent")
	}
	if set.Has("vault.getSecret:privateKey") {
		t.Error("a permission that is not in the set reports as present")
	}
	if (PermissionSet{}).Has("ui.dialogs") {
		t.Error("the empty set reports a permission")
	}
}

// Permissions come back out of the vault as plain strings, so there has to be a way in from them.
// Normalizing here matters for the same reason it does when decoding: a stored list that drifted
// out of order must still compare equal to the one a manifest produces, or every plugin re-prompts.
func TestNewPermissionSetNormalizesItsInput(t *testing.T) {
	set := NewPermissionSet([]string{"ui.dialogs", "auth.provider", "ui.dialogs"})

	want := []string{"auth.provider", "ui.dialogs"}
	if got := set.Tokens(); !slices.Equal(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
}

// The constructor sorts, and sorting in place would reorder a slice its caller is still using -
// here, one read straight out of stored settings.
func TestNewPermissionSetDoesNotDisturbTheCallersSlice(t *testing.T) {
	tokens := []string{"ui.dialogs", "auth.provider"}

	set := NewPermissionSet(tokens)
	tokens[0] = "network.allowArbitraryOutbound"

	if got := set.Tokens(); !slices.Equal(got, []string{"auth.provider", "ui.dialogs"}) {
		t.Fatalf("the set shares its array with the caller: %v", got)
	}
}

// Tokens is what the consent dialog and the audit log render, so a caller must not be able to edit
// the set by writing to the slice it was handed.
func TestTokensHandsOutACopy(t *testing.T) {
	set := PermissionSetFromManifest(&Manifest{
		Capabilities: CapabilitySet{Vault: &VaultCaps{GetSecret: []string{"password"}}},
	})

	set.Tokens()[0] = "vault.getSecret:privateKey"

	if got := set.Tokens(); got[0] != "vault.getSecret:password" {
		t.Fatalf("the set was edited through the slice Tokens returned: %v", got)
	}
}
