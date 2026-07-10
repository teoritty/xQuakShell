package capability

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestGate_DeniesTunnelLocalWithoutProvider(t *testing.T) {
	gate := NewGate(domainplugin.Manifest{ID: "p1"})
	for _, method := range []string{"tunnel.localWrite", "tunnel.localClose", "tunnel.bind", "tunnel.dial"} {
		if gate.Allow(method) {
			t.Fatalf("method %q should be denied without tunnel.provider", method)
		}
	}
}

func TestGate_AllowsTunnelWhenProviderDeclared(t *testing.T) {
	gate := NewGate(domainplugin.Manifest{
		ID: "p1",
		Capabilities: domainplugin.CapabilitySet{
			Tunnel: &domainplugin.TunnelCaps{Provider: true},
		},
	})
	if !gate.Allow("tunnel.localWrite") {
		t.Fatal("expected tunnel.localWrite allowed")
	}
}
