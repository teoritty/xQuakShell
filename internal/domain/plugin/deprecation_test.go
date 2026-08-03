package plugin_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestDeprecationNotices(t *testing.T) {
	// A synthetic registry with a deprecated feature and a fully deprecated capability.
	reg := domainplugin.NewRegistry(map[domainplugin.CapabilityID]domainplugin.CapabilityDescriptor{
		domainplugin.CapVault: {
			Version:  "1.1.0",
			Features: []domainplugin.FeatureID{domainplugin.FeatVaultGetConnection, domainplugin.FeatVaultGetSecret},
			Deprecated: map[domainplugin.FeatureID]domainplugin.DeprecationInfo{
				domainplugin.FeatVaultGetSecret: {Since: "1.1.0", RemoveIn: "2.0.0", Replacement: "vault.getSecretScoped"},
			},
		},
		domainplugin.CapTunnel: {
			Version:  "1.0.0",
			Features: []domainplugin.FeatureID{domainplugin.FeatTunnelDial},
			Deprecated: map[domainplugin.FeatureID]domainplugin.DeprecationInfo{
				domainplugin.FeatureID(""): {Since: "1.0.0", RemoveIn: "2.0.0"},
			},
		},
	})

	nd := domainplugin.NegotiatedDescriptor{
		Capabilities: map[domainplugin.CapabilityID]domainplugin.NegotiatedCapability{
			domainplugin.CapVault:  {Version: mustSemver(t, "1.0.0"), Features: []domainplugin.FeatureID{domainplugin.FeatVaultGetSecret}},
			domainplugin.CapTunnel: {Version: mustSemver(t, "1.0.0"), Features: []domainplugin.FeatureID{domainplugin.FeatTunnelDial}},
		},
	}

	notices := nd.DeprecationNotices(reg)
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
	clean := domainplugin.NegotiatedDescriptor{
		Capabilities: map[domainplugin.CapabilityID]domainplugin.NegotiatedCapability{
			domainplugin.CapVault: {Version: mustSemver(t, "1.0.0"), Features: []domainplugin.FeatureID{domainplugin.FeatVaultGetConnection}},
		},
	}
	if n := clean.DeprecationNotices(reg); len(n) != 0 {
		t.Fatalf("expected no notices, got: %v", n)
	}
}

func mustSemver(t *testing.T, v string) domainplugin.Semver {
	t.Helper()
	s, err := domainplugin.ParseSemver(v)
	if err != nil {
		t.Fatalf("ParseSemver(%q): %v", v, err)
	}
	return s
}
