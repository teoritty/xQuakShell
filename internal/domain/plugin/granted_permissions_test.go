package plugin

import (
	"slices"
	"testing"
)

func allConsent() ConsentFlags {
	return ConsentFlags{
		SecretAccess:     true,
		AuthProvider:     true,
		TunnelProvider:   true,
		MultiSession:     true,
		ArbitraryNetwork: true,
		ExecChannel:      true,
	}
}

func fullManifest() *Manifest {
	return &Manifest{ID: "com.example.sync", Capabilities: fullyPopulatedCapabilities()}
}

// Consenting to everything must grant everything the manifest asked for. If it granted less, an
// install the user fully approved would still be denied at runtime, with nothing to explain why.
func TestConsentingToEverythingGrantsTheWholeManifest(t *testing.T) {
	m := fullManifest()

	granted := GrantedPermissions(m, allConsent())

	if want := PermissionSetFromManifest(m).Tokens(); !slices.Equal(granted.Tokens(), want) {
		t.Fatalf("granted %v, want the full manifest set %v", granted.Tokens(), want)
	}
}

// This is the golden assertion of the whole mapping: with every elevated box unticked, exactly the
// permissions that installing alone confers may remain.
//
// The list is written out rather than computed, deliberately. A new capability field lands in one of
// two places - it is elevated and must vanish when its box is unticked, or it is not and belongs
// here - and either way this test fails until someone decides which. A computed expectation would
// simply absorb the new token and grant it silently.
func TestRefusingEveryElevatedBoxGrantsOnlyWhatInstallingConfers(t *testing.T) {
	granted := GrantedPermissions(fullManifest(), ConsentFlags{})

	want := []string{
		"channel.purposes:exec",
		"config.slots:server",
		"discovery.parentProtocols:ssh",
		"events.publish:com.example.sync/done",
		"events.subscribe:core.session",
		"filesystem.read:${pluginData}/in",
		"filesystem.write:${pluginData}/out",
		"network.outbound:tcp:example.com:443",
		"session.connectProtocols:ssh",
		"session.embed",
		"session.remoteFs",
		"session.terminal",
		"ui.dialogs",
		"ui.nodeDetails",
		"ui.surfaces:terminal",
		"vault.readConnectionFields:host",
	}
	if got := granted.Tokens(); !slices.Equal(got, want) {
		t.Fatalf("refusing everything granted:\n got  %v\n want %v", got, want)
	}
}

// Each box governs its own permissions and nobody else's. A box that leaked into a neighbouring
// group would grant something the user declined, which is the failure this whole mapping exists to
// prevent.
func TestEachConsentBoxGovernsExactlyItsOwnPermissions(t *testing.T) {
	cases := []struct {
		name  string
		set   func(*ConsentFlags)
		gains []string
	}{
		{
			name:  "secret access",
			set:   func(c *ConsentFlags) { c.SecretAccess = true },
			gains: []string{"vault.getSecret:password"},
		},
		{
			name:  "auth provider",
			set:   func(c *ConsentFlags) { c.AuthProvider = true },
			gains: []string{"auth.methods:publickey", "auth.provider"},
		},
		{
			name:  "tunnel provider",
			set:   func(c *ConsentFlags) { c.TunnelProvider = true },
			gains: []string{"tunnel.provider"},
		},
		{
			name:  "multi session",
			set:   func(c *ConsentFlags) { c.MultiSession = true },
			gains: []string{"session.allowMultiSession"},
		},
		{
			name: "arbitrary network",
			set:  func(c *ConsentFlags) { c.ArbitraryNetwork = true },
			// Private networks ride with arbitrary outbound: the manifest field only means anything
			// alongside it, and granting one without the other would record consent for a reach the
			// dialog never offered separately.
			gains: []string{"network.allowArbitraryOutbound", "network.allowPrivateNetworks"},
		},
		{
			name:  "exec channel",
			set:   func(c *ConsentFlags) { c.ExecChannel = true },
			gains: []string{"channel.execCommands:docker ps {host}"},
		},
	}

	baseline := GrantedPermissions(fullManifest(), ConsentFlags{}).Tokens()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			consent := ConsentFlags{}
			tc.set(&consent)

			added, widened := GrantedPermissions(fullManifest(), consent).Widens(
				GrantedPermissions(fullManifest(), ConsentFlags{}))

			if !widened {
				t.Fatalf("ticking %q granted nothing", tc.name)
			}
			if !slices.Equal(added, tc.gains) {
				t.Fatalf("ticking %q added %v, want exactly %v", tc.name, added, tc.gains)
			}
			if len(baseline) == 0 {
				t.Fatal("the baseline is empty, so this comparison proves nothing")
			}
		})
	}
}

// A manifest that never asked for an elevated permission must not gain it from a box the user
// ticked. Consent bounds what was requested; it does not add to it.
func TestConsentCannotGrantWhatTheManifestDidNotRequest(t *testing.T) {
	modest := &Manifest{
		ID: "com.example.modest",
		Capabilities: CapabilitySet{
			Vault: &VaultCaps{ReadConnectionFields: []string{"host"}},
		},
	}

	granted := GrantedPermissions(modest, allConsent())

	if want := []string{"vault.readConnectionFields:host"}; !slices.Equal(granted.Tokens(), want) {
		t.Fatalf("granted %v, want only %v", granted.Tokens(), want)
	}
}

// A plugin asking for nothing elevated is unaffected by the boxes, so installing it grants exactly
// what its manifest says and re-consent never triggers for it.
func TestAModestManifestIsUnaffectedByTheConsentBoxes(t *testing.T) {
	modest := &Manifest{
		ID:           "com.example.modest",
		Capabilities: CapabilitySet{UI: &UICaps{Dialogs: true}},
	}

	refused := GrantedPermissions(modest, ConsentFlags{})
	accepted := GrantedPermissions(modest, allConsent())

	if !slices.Equal(refused.Tokens(), accepted.Tokens()) {
		t.Fatalf("the boxes changed a modest manifest: %v vs %v", refused.Tokens(), accepted.Tokens())
	}
}

// A nil manifest grants nothing rather than panicking on a path that decides plugin permissions.
func TestGrantedPermissionsOfANilManifestIsEmpty(t *testing.T) {
	if !GrantedPermissions(nil, allConsent()).IsEmpty() {
		t.Fatal("a nil manifest produced permissions")
	}
}

// Every elevated token must be claimed by exactly one consent box. Two boxes governing one
// permission would let the user decline it in one dialog and be granted it by the other.
func TestNoElevatedPermissionIsClaimedByTwoBoxes(t *testing.T) {
	seen := map[string]string{}
	boxes := map[string]func(*ConsentFlags){
		"secret":   func(c *ConsentFlags) { c.SecretAccess = true },
		"auth":     func(c *ConsentFlags) { c.AuthProvider = true },
		"tunnel":   func(c *ConsentFlags) { c.TunnelProvider = true },
		"multi":    func(c *ConsentFlags) { c.MultiSession = true },
		"network":  func(c *ConsentFlags) { c.ArbitraryNetwork = true },
		"execChan": func(c *ConsentFlags) { c.ExecChannel = true },
	}
	refused := GrantedPermissions(fullManifest(), ConsentFlags{})

	for box, set := range boxes {
		consent := ConsentFlags{}
		set(&consent)
		added, _ := GrantedPermissions(fullManifest(), consent).Widens(refused)
		for _, token := range added {
			if other, dup := seen[token]; dup {
				t.Errorf("permission %q is granted by both %q and %q", token, other, box)
			}
			seen[token] = box
		}
	}
}
