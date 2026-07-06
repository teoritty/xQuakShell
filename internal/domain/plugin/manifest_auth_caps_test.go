package plugin_test

import (
	"path/filepath"
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func baseAuthManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.example.auth",
		Name:    "Auth",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "auth.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Auth: &domainplugin.AuthCaps{
				Provider: true,
				Methods:  []string{"keyboard-interactive", "publickey"},
			},
		},
		Contributions: domainplugin.Contributions{
			AuthMethods: []domainplugin.AuthMethodContribution{
				{ID: "otp", Label: "OTP", Kind: "keyboard-interactive"},
				{ID: "key", Label: "Key", Kind: "publickey"},
			},
		},
	}
}

func TestValidateAuthCapsAcceptsValidProvider(t *testing.T) {
	m := baseAuthManifest()
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected valid auth manifest: %v", err)
	}
}

func TestValidateAuthCapsRejectsInvalidCapabilityMethod(t *testing.T) {
	m := baseAuthManifest()
	m.Capabilities.Auth.Methods = []string{"password"}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "invalid auth method") {
		t.Fatalf("expected invalid capability method error, got %v", err)
	}
}

func TestValidateAuthCapsProviderRequiresContributions(t *testing.T) {
	m := baseAuthManifest()
	m.Contributions.AuthMethods = nil
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "auth.provider requires") {
		t.Fatalf("expected provider requires contributions error, got %v", err)
	}
}

func TestValidateAuthCapsRejectsUnknownContributionKind(t *testing.T) {
	m := baseAuthManifest()
	m.Contributions.AuthMethods = []domainplugin.AuthMethodContribution{
		{ID: "x", Label: "X", Kind: "password"},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "invalid auth method kind") {
		t.Fatalf("expected invalid kind error, got %v", err)
	}
}

func TestValidateAuthCapsRejectsUnlistedContributionKind(t *testing.T) {
	m := baseAuthManifest()
	m.Capabilities.Auth.Methods = []string{"keyboard-interactive"}
	m.Contributions.AuthMethods = []domainplugin.AuthMethodContribution{
		{ID: "key", Label: "Key", Kind: "publickey"},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "not listed in capabilities.auth.methods") {
		t.Fatalf("expected unlisted kind error, got %v", err)
	}
}

func TestValidateAuthCapsRejectsEmptyContributionID(t *testing.T) {
	m := baseAuthManifest()
	m.Contributions.AuthMethods = []domainplugin.AuthMethodContribution{
		{ID: " ", Label: "Bad", Kind: "keyboard-interactive"},
	}
	if err := m.ValidateCapabilities(); err != domainplugin.ErrInvalidManifest {
		t.Fatalf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestInstalledPluginEntryPath(t *testing.T) {
	p := domainplugin.InstalledPlugin{
		RootDir:  "/plugins/com.example.auth",
		Manifest: domainplugin.Manifest{Engine: domainplugin.EngineConfig{Entry: "bin/auth.exe"}},
	}
	want := filepath.Join("/plugins/com.example.auth", "bin/auth.exe")
	if got := p.EntryPath(); got != want {
		t.Fatalf("EntryPath() = %q", got)
	}
}

func TestGitHubPluginMetadataRequiresSecretAccess(t *testing.T) {
	meta := domainplugin.GitHubPluginMetadata{
		Manifest: domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			},
		},
	}
	if !meta.RequiresSecretAccess() {
		t.Fatal("expected secret access required")
	}
}

func TestManifestSessionSurfaceAndNetworkHelpers(t *testing.T) {
	embed := domainplugin.Manifest{
		Capabilities: domainplugin.CapabilitySet{
			Session: &domainplugin.SessionCaps{Embed: true},
			Network: &domainplugin.NetworkCaps{AllowArbitraryOutbound: true, AllowPrivateNetworks: true},
		},
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{ID: "ssh", EmbedEntry: "ui/ssh.html"},
			},
		},
	}
	if embed.SessionSurface() != "embed" {
		t.Fatalf("SessionSurface() = %q", embed.SessionSurface())
	}
	if embed.EmbedEntryForProtocol("ssh") != "ui/ssh.html" {
		t.Fatalf("EmbedEntryForProtocol() = %q", embed.EmbedEntryForProtocol("ssh"))
	}
	if !embed.RequiresPrivateNetworkAccess() {
		t.Fatal("expected private network access")
	}
}
