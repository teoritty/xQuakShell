package plugin_test

import (
	"crypto/ed25519"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestPermissionSummary(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.perms",
		Name:    "P",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			FS:      &domainplugin.FSCaps{Read: []string{"${pluginData}"}, Write: []string{"${pluginData}"}},
			Network: &domainplugin.NetworkCaps{Outbound: []string{"tcp:127.0.0.1:8080"}},
			Vault:   &domainplugin.VaultCaps{ReadConnectionFields: []string{"host"}, GetSecret: []string{"password"}},
		},
	}
	lines := m.PermissionSummary()
	if len(lines) < 4 {
		t.Fatalf("expected permission lines, got %v", lines)
	}
}

func TestSigningKeyHelpers(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := domainplugin.EncodePublicKey(pub)
	privB64 := domainplugin.EncodePrivateKey(priv)

	keys, err := domainplugin.ParseTrustedPublisherKeys([]string{pubB64})
	if err != nil || len(keys) != 1 {
		t.Fatalf("ParseTrustedPublisherKeys: %v keys=%d", err, len(keys))
	}

	parsedPriv, err := domainplugin.ParsePrivateKey(privB64)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsedPriv) != ed25519.PrivateKeySize {
		t.Fatalf("unexpected private key size %d", len(parsedPriv))
	}
}

func TestValidateSubscribeChannelCoreSessionWildcard(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Subscribe: []string{"core.session.*"},
				Publish:   []string{"plugin.com.example.events.*"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected valid core session subscribe patterns: %v", err)
	}
}

func TestEvaluateInstallTrustUnsignedWarning(t *testing.T) {
	m := domainplugin.Manifest{
		ID: "com.unsigned", Name: "U", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "u.exe"},
	}
	trust, err := domainplugin.EvaluateInstallTrust(m, domainplugin.InstallTrustPolicy{})
	if err != nil || !trust.UnsignedWarning {
		t.Fatalf("expected unsigned warning: %+v err=%v", trust, err)
	}
}

func TestValidatePluginEventPatternOwnPublishWildcard(t *testing.T) {
	if err := domainplugin.ValidatePluginEventPattern("com.example.a", "plugin.com.example.a.status.*"); err != nil {
		t.Fatalf("expected valid publish wildcard: %v", err)
	}
}
