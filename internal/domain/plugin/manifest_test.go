package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestManifestValidate(t *testing.T) {
	valid := domainplugin.Manifest{
		ID:      "com.example.echo",
		Name:    "Echo",
		Version: "1.0.0",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "echo.exe",
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid manifest: %v", err)
	}
}

func TestManifestValidateMissingID(t *testing.T) {
	m := domainplugin.Manifest{Name: "Echo", Version: "1.0.0", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x"}}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestRequiresSecretAccess(t *testing.T) {
	m := domainplugin.Manifest{
		Capabilities: domainplugin.CapabilitySet{
			Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
		},
	}
	if !m.RequiresSecretAccess() {
		t.Fatal("expected secret access requirement")
	}
}

func TestEffectiveIsolationDefault(t *testing.T) {
	m := domainplugin.Manifest{}
	if m.EffectiveIsolation() != domainplugin.IsolationPerPlugin {
		t.Fatalf("expected per-plugin default, got %q", m.EffectiveIsolation())
	}
}
