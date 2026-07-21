package capability

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestGate_VersionEnforcementDeniesBelowNegotiated proves the forward-looking feature-version
// mechanism: a method introduced above baseline is denied for a plugin whose negotiated capability
// version does not reach it, even though the capability is granted. Both the synthetic
// featureVersions entry and the negotiated descriptors are constructed by hand, since at the 1.0
// freeze every method is baseline and the host provides only 1.0.0 (ADR-012 edge #13).
func TestGate_VersionEnforcementDeniesBelowNegotiated(t *testing.T) {
	manifest := domainplugin.Manifest{
		ID:           "p1",
		Capabilities: domainplugin.CapabilitySet{Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}}},
	}
	future := mustParse(t, "1.2.0")
	baseline := mustParse(t, "1.0.0")

	// Negotiated vault 1.0.0, but vault.getSecret is pinned to a hypothetical 1.2.0: the grant
	// permits it but the negotiated version is too low, so it is denied.
	low := NewGate(manifest, domainplugin.NegotiatedDescriptor{
		PluginAPI:    baseline,
		Capabilities: map[domainplugin.CapabilityID]domainplugin.NegotiatedCapability{domainplugin.CapVault: {Version: baseline}},
	})
	low.featureVersions["vault.getSecret"] = methodFeature{capability: domainplugin.CapVault, minVersion: future}
	if low.Allow("vault.getSecret") {
		t.Fatal("expected vault.getSecret denied for a plugin that negotiated vault 1.0.0")
	}

	// Negotiated vault 1.2.0: the same pinned method is now allowed (grant present, version met).
	high := NewGate(manifest, domainplugin.NegotiatedDescriptor{
		PluginAPI:    baseline,
		Capabilities: map[domainplugin.CapabilityID]domainplugin.NegotiatedCapability{domainplugin.CapVault: {Version: future}},
	})
	high.featureVersions["vault.getSecret"] = methodFeature{capability: domainplugin.CapVault, minVersion: future}
	if !high.Allow("vault.getSecret") {
		t.Fatal("expected vault.getSecret allowed once vault 1.2.0 is negotiated")
	}
}

func mustParse(t *testing.T, v string) domainplugin.Semver {
	t.Helper()
	s, err := domainplugin.ParseSemver(v)
	if err != nil {
		t.Fatalf("ParseSemver(%q): %v", v, err)
	}
	return s
}
