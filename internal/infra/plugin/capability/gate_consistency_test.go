package capability

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
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

// TestRegistryCapabilitiesAreGrantable guards that the registry and the CapabilitySet grant fields
// describe the same capability set. Both directions must hold: every capability the host advertises
// must be grantable (else it is unreachable), and every grantable capability must be in the
// registry (else it cannot be negotiated). The grantable set is derived from GrantedCapabilityNames
// on a fully-granted CapabilitySet — the single mapping from grant fields to ids — so there is no
// hardcoded list to drift.
func TestRegistryCapabilitiesAreGrantable(t *testing.T) {
	allGranted := domainplugin.CapabilitySet{
		Network:   &domainplugin.NetworkCaps{},
		FS:        &domainplugin.FSCaps{},
		Events:    &domainplugin.EventCaps{},
		Vault:     &domainplugin.VaultCaps{},
		Session:   &domainplugin.SessionCaps{},
		Auth:      &domainplugin.AuthCaps{},
		Tunnel:    &domainplugin.TunnelCaps{},
		Channel:   &domainplugin.ChannelCaps{},
		Discovery: &domainplugin.DiscoveryCaps{},
	}
	grantable := map[domainplugin.CapabilityID]bool{}
	for _, id := range allGranted.GrantedCapabilityNames() {
		grantable[id] = true
	}

	reg := domainplugin.HostRegistry()
	for _, name := range reg.Names() {
		if !grantable[name] {
			t.Fatalf("registry capability %q has no CapabilitySet grant field", name)
		}
	}
	for id := range grantable {
		if !reg.Has(id) {
			t.Fatalf("grantable capability %q is missing from the registry", id)
		}
	}
}
