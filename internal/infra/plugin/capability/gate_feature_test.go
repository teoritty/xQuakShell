package capability

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TestGate_VersionEnforcementDeniesBelowNegotiated proves the forward-looking feature-version
// mechanism: a method introduced above baseline is denied for a plugin whose negotiated capability
// version does not reach it, even though the capability is granted. A synthetic featureVersions
// entry stands in for a future minor's method, since every method is baseline at the 1.0 freeze
// (ADR-012 edge #13). The gate derives the negotiated version from the manifest itself.
func TestGate_VersionEnforcementDeniesBelowNegotiated(t *testing.T) {
	future, _ := domainplugin.ParseSemver("1.2.0")

	// Plugin grants vault but declares no requires → negotiated vault baseline 1.0.0. Pin
	// vault.getSecret to a hypothetical 1.2.0: the grant permits it but the negotiated version is
	// too low, so it is denied.
	low := NewGate(domainplugin.Manifest{
		ID:           "p1",
		Capabilities: domainplugin.CapabilitySet{Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}}},
	})
	low.featureVersions["vault.getSecret"] = methodFeature{capability: "vault", minVersion: future}
	if low.Allow("vault.getSecret") {
		t.Fatal("expected vault.getSecret denied for a plugin that negotiated vault 1.0.0")
	}

	// Plugin explicitly requires vault 1.2.0 → negotiated vault 1.2.0. The same pinned method is
	// now allowed (grant present and version satisfied).
	high := NewGate(domainplugin.Manifest{
		ID:           "p1",
		Capabilities: domainplugin.CapabilitySet{Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}}},
		Requires: &domainplugin.RequirementSet{
			PluginAPI:    "1.0.0",
			Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{"vault": {Min: "1.2.0"}},
		},
	})
	high.featureVersions["vault.getSecret"] = methodFeature{capability: "vault", minVersion: future}
	if !high.Allow("vault.getSecret") {
		t.Fatal("expected vault.getSecret allowed once vault 1.2.0 is negotiated")
	}
}
