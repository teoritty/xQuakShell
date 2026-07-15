package plugin_test

import (
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestDeprecationNotices(t *testing.T) {
	// A synthetic registry with a deprecated feature and a fully deprecated capability.
	reg := domainplugin.Registry{
		"vault": {
			Version:  "1.1.0",
			Features: []string{"getConnection", "getSecret"},
			Deprecated: map[string]domainplugin.DeprecationInfo{
				"getSecret": {Since: "1.1.0", RemoveIn: "2.0.0", Replacement: "vault.getSecretScoped"},
			},
		},
		"tunnel": {
			Version:  "1.0.0",
			Features: []string{"dial"},
			Deprecated: map[string]domainplugin.DeprecationInfo{
				"": {Since: "1.0.0", RemoveIn: "2.0.0"},
			},
		},
	}

	rs := domainplugin.RequirementSet{
		PluginAPI: "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{
			"vault":  {Min: "1.0.0", Features: []string{"getSecret"}},
			"tunnel": {Min: "1.0.0", Features: []string{"dial"}},
		},
	}

	notices := rs.DeprecationNotices(reg)
	joined := strings.Join(notices, "\n")
	if !strings.Contains(joined, "vault.getSecret is deprecated") {
		t.Fatalf("expected vault.getSecret deprecation notice, got: %v", notices)
	}
	if !strings.Contains(joined, "vault.getSecretScoped") {
		t.Fatalf("expected replacement hint, got: %v", notices)
	}
	if !strings.Contains(joined, "tunnel is deprecated") {
		t.Fatalf("expected whole-capability tunnel deprecation notice, got: %v", notices)
	}

	// A requirement using no deprecated items produces no notices.
	clean := domainplugin.RequirementSet{
		PluginAPI:    "1.0.0",
		Capabilities: map[string]domainplugin.CapabilityRequirement{"vault": {Min: "1.0.0", Features: []string{"getConnection"}}},
	}
	if n := clean.DeprecationNotices(reg); len(n) != 0 {
		t.Fatalf("expected no notices, got: %v", n)
	}
}
