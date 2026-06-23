package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestParseNetworkPatternRejectsPortOnly(t *testing.T) {
	cases := []string{"tcp:443", "tcp:*", "tcp:", "443", "tcp:example.com"}
	for _, pattern := range cases {
		if _, err := domainplugin.ParseNetworkPattern(pattern); err == nil {
			t.Fatalf("expected reject for pattern %q", pattern)
		}
	}
}

func TestParseNetworkPatternAcceptsHostPort(t *testing.T) {
	cases := []string{
		"tcp:127.0.0.1:443",
		"tcp:example.com:22",
		"tcp:localhost:8000-9000",
	}
	for _, pattern := range cases {
		if _, err := domainplugin.ParseNetworkPattern(pattern); err != nil {
			t.Fatalf("expected accept for %q: %v", pattern, err)
		}
	}
}

func TestValidateManifestRejectsPortOnlyNetwork(t *testing.T) {
	m := domainplugin.Manifest{
		ID: "com.test.portonly", Name: "x", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Network: &domainplugin.NetworkCaps{Outbound: []string{"tcp:443"}},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected tcp:443 to be rejected at manifest validation")
	}
}
