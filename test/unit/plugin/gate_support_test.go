package plugin_test

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
)

// newGate builds a capability gate the way the runtime does — resolving the manifest against the
// host registry into a NegotiatedDescriptor — so tests exercise the real NewGate(manifest, nd)
// path. It fails loudly if negotiation errors rather than silently constructing an empty contract.
//
// A manifest that declares no requires{} is pinned to what this host currently provides. Gate tests
// are about which methods a capability authorises, not about version negotiation: without this, a
// major bump on any capability would rewrite a dozen tests that never mentioned a version. A test
// that means to exercise negotiation declares its own requires{} and keeps it.
func newGate(t *testing.T, m domainplugin.Manifest) *capability.Gate {
	t.Helper()
	if m.Requires == nil {
		m.Requires = currentHostRequirements(m)
	}
	nd, _, err := domainplugin.Negotiate(&m, domainplugin.HostRegistry())
	if err != nil {
		t.Fatalf("negotiate manifest %q: %v", m.ID, err)
	}
	return capability.NewGate(m, nd)
}

// currentHostRequirements requires, for every capability the manifest grants, exactly the version
// this host offers.
func currentHostRequirements(m domainplugin.Manifest) *domainplugin.RequirementSet {
	reg := domainplugin.HostRegistry()
	caps := map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{}
	for _, name := range m.Capabilities.GrantedCapabilityNames() {
		if v, ok := reg.CapabilityVersion(name); ok {
			caps[name] = domainplugin.CapabilityRequirement{Min: v.String()}
		}
	}
	return &domainplugin.RequirementSet{PluginAPI: domainplugin.PluginAPIVersion, Capabilities: caps}
}
