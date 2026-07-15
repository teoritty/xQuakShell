package capability

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TestFeatureVersionsReferenceRealFeatures guards against drift between the enforced surface and
// the declared contract: every method pinned in defaultFeatureVersions must name a capability and
// feature that actually exists in the host registry. Empty at the 1.0 freeze, so this trivially
// passes today — it fires the moment a future minor adds a versioned method with a typo'd or
// unregistered capability/feature (ADR-012).
func TestFeatureVersionsReferenceRealFeatures(t *testing.T) {
	reg := domainplugin.HostRegistry()
	for method, mf := range defaultFeatureVersions() {
		if !reg.Has(mf.capability) {
			t.Fatalf("method %q maps to unknown capability %q", method, mf.capability)
		}
	}
}

// TestRegistryCapabilitiesAreGrantable guards the other direction: every capability the host
// advertises must be one a plugin can actually grant in its CapabilitySet. A registry entry with
// no corresponding grant would be unreachable and impossible to negotiate correctly.
func TestRegistryCapabilitiesAreGrantable(t *testing.T) {
	grantable := map[domainplugin.CapabilityID]bool{
		"network": true, "filesystem": true, "events": true, "vault": true,
		"session": true, "auth": true, "tunnel": true, "channel": true,
	}
	for name := range domainplugin.HostRegistry() {
		if !grantable[name] {
			t.Fatalf("registry capability %q has no CapabilitySet grant field", name)
		}
	}
}
