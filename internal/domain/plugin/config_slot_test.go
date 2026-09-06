package plugin

import (
	"errors"
	"slices"
	"testing"
)

func endpointSlot(schemes ...string) ConfigSlot {
	return ConfigSlot{ID: "server", Label: "Sync server", Kind: SlotKindEndpoint, Schemes: schemes, Required: true}
}

func slotManifest(slots ...ConfigSlot) *Manifest {
	return &Manifest{
		ID:           "com.example.sync",
		Capabilities: CapabilitySet{Config: &ConfigCaps{Slots: slots}},
	}
}

// The whole point of slots: a plugin whose destination only the user knows gets there without
// asking for arbitrary outbound, which is the widest grant in the system and the one a credential
// plugin should never have to request.
func TestAFilledEndpointBecomesAnOutboundPattern(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	cfg := PluginConfig{Values: map[string]string{"server": "https://sync.example.com"}}

	patterns, err := EffectiveOutbound(m, cfg)
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v, want nil", err)
	}

	if !slices.Contains(patterns, "tcp:sync.example.com:443") {
		t.Fatalf("patterns = %v, want the endpoint as tcp:sync.example.com:443", patterns)
	}
}

// The derived pattern has to be one the existing network gate accepts, or the slot grants a
// destination nothing can dial and the plugin fails with no explanation.
func TestADerivedPatternParsesAsANetworkPattern(t *testing.T) {
	m := slotManifest(endpointSlot("https", "http"))
	cfg := PluginConfig{Values: map[string]string{"server": "http://10.0.0.5:8443/api"}}

	patterns, err := EffectiveOutbound(m, cfg)
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v", err)
	}

	for _, pattern := range patterns {
		if _, err := ParseNetworkPattern(pattern); err != nil {
			t.Fatalf("derived pattern %q is not one the gate accepts: %v", pattern, err)
		}
	}
	if !slices.Contains(patterns, "tcp:10.0.0.5:8443") {
		t.Fatalf("patterns = %v, want the explicit port carried through", patterns)
	}
}

// The manifest's own patterns are not replaced by the slots. A plugin that names a fixed host and
// also takes one from the user needs both.
func TestManifestPatternsSurviveAlongsideTheSlots(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	m.Capabilities.Network = &NetworkCaps{Outbound: []string{"tcp:fixed.example.com:443"}}
	cfg := PluginConfig{Values: map[string]string{"server": "https://sync.example.com"}}

	patterns, err := EffectiveOutbound(m, cfg)
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v", err)
	}

	for _, want := range []string{"tcp:fixed.example.com:443", "tcp:sync.example.com:443"} {
		if !slices.Contains(patterns, want) {
			t.Errorf("%q is missing from %v", want, patterns)
		}
	}
}

// A scheme the slot does not declare must be refused. The declared list is what the plugin author
// said their protocol is, and accepting anything else would let a typed value change what the
// plugin talks to.
func TestASchemeTheSlotDoesNotDeclareIsRefused(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	cfg := PluginConfig{Values: map[string]string{"server": "http://sync.example.com"}}

	if _, err := EffectiveOutbound(m, cfg); !errors.Is(err, ErrInvalidPluginConfig) {
		t.Fatalf("err = %v, want ErrInvalidPluginConfig for an undeclared scheme", err)
	}
}

// Credentials in a configuration value would be shown in the settings panel, written to the audit
// trail and carried in every log line that names the endpoint. A secret belongs in PluginSecrets,
// not here, and the refusal is what keeps the two apart.
func TestAnEndpointCarryingCredentialsIsRefused(t *testing.T) {
	m := slotManifest(endpointSlot("https"))

	for _, value := range []string{
		"https://user@sync.example.com",
		"https://user:hunter2@sync.example.com",
	} {
		cfg := PluginConfig{Values: map[string]string{"server": value}}
		if _, err := EffectiveOutbound(m, cfg); !errors.Is(err, ErrInvalidPluginConfig) {
			t.Errorf("%q: err = %v, want ErrInvalidPluginConfig", value, err)
		}
	}
}

// The pattern format splits on the first colon, so it cannot express an IPv6 literal at all.
// Refusing is the honest answer: deriving one anyway would produce a pattern that parses into a
// different host than the user typed.
func TestAnIPv6EndpointIsRefusedRatherThanMangled(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	cfg := PluginConfig{Values: map[string]string{"server": "https://[2001:db8::1]:8443"}}

	if _, err := EffectiveOutbound(m, cfg); !errors.Is(err, ErrInvalidPluginConfig) {
		t.Fatalf("err = %v, want ErrInvalidPluginConfig for an IPv6 literal", err)
	}
}

// Values that are not endpoints at all must be refused rather than silently contributing nothing:
// a plugin that cannot reach its server should say so, not fail to dial for reasons nobody can see.
func TestAValueThatIsNotAnEndpointIsRefused(t *testing.T) {
	m := slotManifest(endpointSlot("https"))

	for _, value := range []string{
		"sync.example.com",
		"https://",
		"https://sync.example.com:0",
		"https://sync.example.com:70000",
		"not a url at all",
	} {
		cfg := PluginConfig{Values: map[string]string{"server": value}}
		if _, err := EffectiveOutbound(m, cfg); !errors.Is(err, ErrInvalidPluginConfig) {
			t.Errorf("%q: err = %v, want ErrInvalidPluginConfig", value, err)
		}
	}
}

// A value left over from a slot the manifest no longer declares grants nothing. A plugin update
// that drops a slot must not keep reaching the host it used to name.
func TestAValueForASlotTheManifestDroppedGrantsNothing(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	cfg := PluginConfig{Values: map[string]string{
		"server":  "https://sync.example.com",
		"retired": "https://old.example.com",
	}}

	patterns, err := EffectiveOutbound(m, cfg)
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v", err)
	}

	if slices.Contains(patterns, "tcp:old.example.com:443") {
		t.Fatalf("a value for a slot that no longer exists still granted a destination: %v", patterns)
	}
}

// An unfilled slot contributes nothing and is not an error here. Whether a required slot must be
// filled before the plugin runs is a question for the UI; this function answers only what the user
// has actually allowed so far.
func TestAnUnfilledSlotContributesNothing(t *testing.T) {
	m := slotManifest(endpointSlot("https"))

	patterns, err := EffectiveOutbound(m, PluginConfig{})
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v, want nil", err)
	}
	if len(patterns) != 0 {
		t.Fatalf("patterns = %v, want none", patterns)
	}
}

// A manifest with no slots behaves exactly as it did before slots existed.
func TestAManifestWithNoSlotsIsUnchanged(t *testing.T) {
	m := &Manifest{
		ID:           "com.example.plain",
		Capabilities: CapabilitySet{Network: &NetworkCaps{Outbound: []string{"tcp:fixed.example.com:443"}}},
	}

	patterns, err := EffectiveOutbound(m, PluginConfig{})
	if err != nil {
		t.Fatalf("EffectiveOutbound err = %v", err)
	}
	if !slices.Equal(patterns, []string{"tcp:fixed.example.com:443"}) {
		t.Fatalf("patterns = %v, want the manifest's own list unchanged", patterns)
	}
}

// Slot declarations are part of the manifest contract, so they are validated when it loads rather
// than when a value is typed - a plugin whose slots are malformed must not install at all.
func TestSlotDeclarationsAreValidated(t *testing.T) {
	cases := []struct {
		name  string
		slots []ConfigSlot
	}{
		{"no id", []ConfigSlot{{Label: "S", Kind: SlotKindEndpoint, Schemes: []string{"https"}}}},
		{"no label", []ConfigSlot{{ID: "server", Kind: SlotKindEndpoint, Schemes: []string{"https"}}}},
		// Schemes are supplied here so that only the kind check can reject this one. Without them the
		// case is caught by "an endpoint declares no scheme" instead, and the kind check could be
		// deleted with every test still green.
		{"unknown kind", []ConfigSlot{{ID: "server", Label: "S", Kind: "freeform", Schemes: []string{"https"}}}},
		{"endpoint with no scheme", []ConfigSlot{{ID: "server", Label: "S", Kind: SlotKindEndpoint}}},
		{"unsupported scheme", []ConfigSlot{{ID: "server", Label: "S", Kind: SlotKindEndpoint, Schemes: []string{"ftp"}}}},
		{"duplicate ids", []ConfigSlot{endpointSlot("https"), endpointSlot("https")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := slotManifest(tc.slots...).ValidateCapabilities(); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("err = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

// A well-formed declaration must pass, or the validation above would be indistinguishable from
// refusing slots altogether.
func TestAWellFormedSlotDeclarationIsAccepted(t *testing.T) {
	if err := slotManifest(endpointSlot("https", "wss")).ValidateCapabilities(); err != nil {
		t.Fatalf("a well-formed slot declaration was refused: %v", err)
	}
}

// A slot is a permission: it is a destination the plugin may reach that its manifest never named,
// so a new one must reach the user rather than arriving with an update.
func TestASlotIsAPermission(t *testing.T) {
	before := PermissionSetFromManifest(slotManifest(endpointSlot("https")))
	after := PermissionSetFromManifest(slotManifest(
		endpointSlot("https"),
		ConfigSlot{ID: "mirror", Label: "Mirror", Kind: SlotKindEndpoint, Schemes: []string{"https"}},
	))

	added, widened := after.Widens(before)
	if !widened {
		t.Fatal("a manifest that added a configuration slot did not widen its permissions")
	}
	if len(added) != 1 || added[0] != "config.slots:mirror" {
		t.Fatalf("added = %v, want exactly [config.slots:mirror]", added)
	}
}

// A nil manifest allows nothing rather than panicking. This is called from the dial path, where a
// manifest that failed to load is an ordinary state and a crash is not.
func TestEffectiveOutboundOfANilManifestAllowsNothing(t *testing.T) {
	patterns, err := EffectiveOutbound(nil, PluginConfig{Values: map[string]string{"server": "https://x"}})
	if err != nil || len(patterns) != 0 {
		t.Fatalf("patterns = %v, err = %v, want none and nil", patterns, err)
	}
}

// Most malformed values still parse as a URL and are caught by the scheme check, which is why the
// parser's own failure needs a value of its own: a host with a space is rejected by net/url before
// anything here gets to look at it.
func TestAValueTheURLParserRejectsIsRefused(t *testing.T) {
	m := slotManifest(endpointSlot("https"))
	cfg := PluginConfig{Values: map[string]string{"server": "https://sync example.com"}}

	if _, err := EffectiveOutbound(m, cfg); !errors.Is(err, ErrInvalidPluginConfig) {
		t.Fatalf("err = %v, want ErrInvalidPluginConfig for a value net/url refuses", err)
	}
}
