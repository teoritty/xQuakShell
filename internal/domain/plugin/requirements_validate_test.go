package plugin_test

import (
	"encoding/json"
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
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

	// Same manifest but requiring a feature the host does not offer must fail Validate.
	bad := m
	bad.Requires = &domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"mindRead"}}},
	}
	if err := bad.Validate(); !errors.Is(err, domainplugin.ErrMissingFeature) {
		t.Fatalf("expected ErrMissingFeature, got %v", err)
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
		{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0-dev"}}},                     // cap pre-release
		{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{""}}}}, // empty feature
	}
	for i, req := range bad {
		m := vaultManifest(req)
		if err := m.ValidateRequirements(); err == nil {
			t.Fatalf("case %d: expected invalid requirements, got nil", i)
		}
	}

	good := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"getSecret"}}},
	})
	if err := good.ValidateRequirements(); err != nil {
		t.Fatalf("expected valid requirements, got %v", err)
	}
}

func TestValidateRequirementsRejectsUngrantedCapability(t *testing.T) {
	// Requires "network" but only grants vault (edge #2).
	m := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"network": {Min: "1.0.0"}},
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
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"getSecret"}}}},
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
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.3.0"}}},
			compatible: false,
			is:         domainplugin.ErrIncompatibleAPI,
		},
		{
			name:       "unknown capability rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"telepathy": {Min: "1.0.0"}}},
			compatible: false,
			is:         domainplugin.ErrIncompatibleAPI,
		},
		{
			name:       "missing feature rejected",
			req:        domainplugin.RequirementSet{PluginAPI: "1.0.0", Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"mindRead"}}}},
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

func TestEffectiveRequirementsMigration(t *testing.T) {
	// Legacy pre-1.0 minCoreVersion is rejected (edge #8).
	pre := vaultManifest(nil)
	pre.MinCoreVersion = "0.2.0"
	if _, _, err := domainplugin.EffectiveRequirements(&pre); !errors.Is(err, domainplugin.ErrIncompatibleAPI) {
		t.Fatalf("expected ErrIncompatibleAPI for pre-1.0 minCoreVersion, got %v", err)
	}

	// Legacy >=1.0 minCoreVersion migrates with a deprecation warning (edge #9).
	legacy := vaultManifest(nil)
	legacy.MinCoreVersion = "1.0.0"
	eff, warns, err := domainplugin.EffectiveRequirements(&legacy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff.PluginAPI != "1.0.0" {
		t.Fatalf("expected migrated pluginApi 1.0.0, got %q", eff.PluginAPI)
	}
	if len(warns) == 0 {
		t.Fatal("expected a deprecation warning")
	}
	// Implicit baseline requirement injected for the granted vault capability (edge #3).
	if _, ok := eff.Capabilities["vault"]; !ok {
		t.Fatal("expected implicit vault baseline requirement")
	}

	// No requires + no minCoreVersion defaults to current envelope (edge #1).
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

	// requires{} present alongside minCoreVersion: requires wins, warns (edge #10).
	both := vaultManifest(&domainplugin.RequirementSet{PluginAPI: "1.0.0"})
	both.MinCoreVersion = "1.0.0"
	effBoth, warnsBoth, err := domainplugin.EffectiveRequirements(&both)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effBoth.PluginAPI != "1.0.0" || len(warnsBoth) == 0 {
		t.Fatalf("expected requires-wins with warning, got api=%q warns=%v", effBoth.PluginAPI, warnsBoth)
	}
}

func TestManifestValidateIntegratesRequirements(t *testing.T) {
	// A fully valid manifest with satisfiable requires passes end-to-end.
	m := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"getSecret"}}},
	})
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}

	// An unsatisfiable feature requirement fails Validate.
	bad := vaultManifest(&domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"mindRead"}}},
	})
	if err := bad.Validate(); !errors.Is(err, domainplugin.ErrMissingFeature) {
		t.Fatalf("expected ErrMissingFeature from Validate, got %v", err)
	}
}
