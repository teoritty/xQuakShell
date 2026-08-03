package plugin_test

import (
	"encoding/json"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestRequiresJSONRoundTrip proves the requires{} block deserialises from the on-the-wire
// plugin.json shape and drives validation end-to-end.
func TestRequiresJSONRoundTrip(t *testing.T) {
	raw := `{
      "id":"com.example.req","name":"Req","version":"1.0.0",
      "engine":{"type":"go-binary","entry":"bin/plugin"},
      "capabilities":{"vault":{"getSecret":["password"]}},
      "requires":{"pluginApi":"1.0.0","capabilities":{"vault":{"min":"1.0.0","features":["getSecret"]}}}
    }`
	var m domainplugin.Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Requires == nil || m.Requires.PluginAPI != "1.0.0" {
		t.Fatalf("requires not parsed: %+v", m.Requires)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}

	// Same manifest but requiring a feature the host does not offer is still well-formed (passes
	// Validate) and is rejected only by the host compatibility gate.
	bad := m
	bad.Requires = &domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"mindRead"}}},
	}
	if err := bad.Validate(); err != nil {
		t.Fatalf("expected structural validation to pass, got %v", err)
	}
	if err := bad.CheckHostCompatibility(); !errors.Is(err, domainplugin.ErrMissingFeature) {
		t.Fatalf("expected ErrMissingFeature from CheckHostCompatibility, got %v", err)
	}
}

// vaultManifest builds a minimal valid manifest that grants the vault capability, optionally
// with a requires{} block.
func vaultManifest(req *domainplugin.RequirementSet) domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:           "com.test.example",
		Name:         "Example",
		Version:      "1.0.0",
		Engine:       domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "bin/plugin"},
		Capabilities: domainplugin.CapabilitySet{Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}}},
		Requires:     req,
	}
}

func TestValidateRequirementsGrammar(t *testing.T) {
	bad := []*domainplugin.RequirementSet{
		{PluginAPI: ""},          // missing pluginApi
		{PluginAPI: "1.0"},       // not full semver
		{PluginAPI: "1.0.0-rc1"}, // pre-release not allowed in a requirement
		{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0-dev"}}},                                     // cap pre-release
		{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{""}}}}, // empty feature
	}
	for i, req := range bad {
		m := vaultManifest(req)
		if err := m.ValidateRequirements(); err == nil {
			t.Fatalf("case %d: expected invalid requirements, got nil", i)
		}
	}

	good := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"getSecret"}}},
	})
	if err := good.ValidateRequirements(); err != nil {
		t.Fatalf("expected valid requirements, got %v", err)
	}
}

func TestValidateRequirementsRejectsUngrantedCapability(t *testing.T) {
	// Requires "network" but only grants vault (edge #2).
	m := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"network": {Min: "1.0.0"}},
	})
	if err := m.ValidateRequirements(); err == nil {
		t.Fatal("expected rejection for requiring an ungranted capability")
	}
}

func TestCheckAgainstHostMatrix(t *testing.T) {
	reg := domainplugin.HostRegistry() // everything at 1.0.0

	cases := []struct {
		name       string
		req        domainplugin.RequirementSet
		compatible bool
		is         error
	}{
		{
			name:       "exact baseline",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"getSecret"}}}},
			compatible: true,
		},
		{
			name:       "newer major envelope rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "2.0.0"},
			compatible: false,
			is:         domainplugin.ErrIncompatibleAPI,
		},
		{
			name:       "higher minor than host rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.3.0"}}},
			compatible: false,
			is:         domainplugin.ErrIncompatibleAPI,
		},
		{
			name:       "unknown capability rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"telepathy": {Min: "1.0.0"}}},
			compatible: false,
			is:         domainplugin.ErrIncompatibleAPI,
		},
		{
			name:       "missing feature rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"mindRead"}}}},
			compatible: false,
			is:         domainplugin.ErrMissingFeature,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := tc.req.CheckAgainstHost(reg)
			if tc.compatible {
				if report != nil {
					t.Fatalf("expected compatible, got %v", report)
				}
				return
			}
			if report == nil {
				t.Fatal("expected incompatibility report, got nil")
			}
			if tc.is != nil && !errors.Is(report, tc.is) {
				t.Fatalf("expected errors.Is %v, got %v", tc.is, report)
			}
		})
	}
}

func TestEffectiveRequirementsDefaults(t *testing.T) {
	// No requires{} defaults to current envelope (edge #1).
	bare := vaultManifest(nil)
	effBare, warnsBare, err := domainplugin.EffectiveRequirements(&bare)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effBare.PluginAPI != domainplugin.PluginAPIVersion {
		t.Fatalf("expected default pluginApi %q, got %q", domainplugin.PluginAPIVersion, effBare.PluginAPI)
	}
	if len(warnsBare) == 0 {
		t.Fatal("expected advisory warning for missing requires{}")
	}
	// Implicit baseline requirement injected for the granted vault capability (edge #3).
	if _, ok := effBare.Capabilities["vault"]; !ok {
		t.Fatal("expected implicit vault baseline requirement")
	}

	// minCoreVersion is no longer supported: hard rejection.
	legacy := vaultManifest(nil)
	legacy.MinCoreVersion = "1.0.0"
	if _, _, err := domainplugin.EffectiveRequirements(&legacy); !errors.Is(err, domainplugin.ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest for minCoreVersion, got %v", err)
	}
}

func TestManifestValidateIntegratesRequirements(t *testing.T) {
	// A fully valid manifest with satisfiable requires passes both structural validation and the
	// host compatibility gate.
	m := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"getSecret"}}},
	})
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}
	if err := m.CheckHostCompatibility(); err != nil {
		t.Fatalf("expected compatible manifest, got %v", err)
	}

	// An unsatisfiable feature requirement is well-formed (passes Validate) but is caught by the
	// host compatibility gate — parsing/listing stays tolerant; gating is separate (ADR-012).
	bad := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []domainplugin.FeatureID{"mindRead"}}},
	})
	if err := bad.Validate(); err != nil {
		t.Fatalf("expected bad manifest to pass structural validation, got %v", err)
	}
	if err := bad.CheckHostCompatibility(); !errors.Is(err, domainplugin.ErrMissingFeature) {
		t.Fatalf("expected ErrMissingFeature from CheckHostCompatibility, got %v", err)
	}
}
