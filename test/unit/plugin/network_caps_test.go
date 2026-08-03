package plugin_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func baseManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID: "com.test.net", Name: "Net", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
	}
}

func TestValidateNetworkCapsAllowArbitraryWithoutOutbound(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{AllowArbitraryOutbound: true}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest: %v", err)
	}
}

func TestValidateNetworkCapsRejectPrivateWithoutArbitrary(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{AllowPrivateNetworks: true}
	if err := m.Validate(); err == nil {
		t.Fatal("expected allowPrivateNetworks without allowArbitraryOutbound to fail")
	}
}

func TestValidateNetworkCapsStillRejectsWildcards(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		Outbound:               []string{"tcp:*:443"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected wildcard outbound to be rejected")
	}
}

func TestRequiresArbitraryNetworkAccess(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{AllowArbitraryOutbound: true}
	if !m.RequiresArbitraryNetworkAccess() {
		t.Fatal("expected RequiresArbitraryNetworkAccess true")
	}
	if !m.RequiresArbitraryNetworkWarning() {
		t.Fatal("expected RequiresArbitraryNetworkWarning true")
	}
	if !m.HasNetworkCapability() {
		t.Fatal("expected HasNetworkCapability true")
	}
}

func TestPermissionSummaryArbitraryNetwork(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}
	lines := m.PermissionSummary()
	found := false
	for _, line := range lines {
		if strings.Contains(line, "any public host and port") && strings.Contains(line, "private") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected arbitrary network permission line, got %v", lines)
	}
}
