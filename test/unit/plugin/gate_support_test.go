package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

// newGate builds a capability gate the way the runtime does — resolving the manifest against the
// host registry into a NegotiatedDescriptor — so tests exercise the real NewGate(manifest, nd)
// path. It fails loudly if negotiation errors rather than silently constructing an empty contract.
func newGate(t *testing.T, m domainplugin.Manifest) *capability.Gate {
	t.Helper()
	nd, _, err := domainplugin.Negotiate(&m, domainplugin.HostRegistry())
	if err != nil {
		t.Fatalf("negotiate manifest %q: %v", m.ID, err)
	}
	return capability.NewGate(m, nd)
}
