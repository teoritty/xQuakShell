package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/bundle"
	infraplugin "ssh-client/internal/infra/plugin"
)

func TestValidateIDRejectsUnsafe(t *testing.T) {
	cases := []string{"", "..", "../evil", "com/evil", `com\evil`, "UPPER.case"}
	for _, id := range cases {
		if err := domainplugin.ValidateID(id); err == nil {
			t.Fatalf("expected reject for id %q", id)
		}
	}
	if err := domainplugin.ValidateID("com.example.valid-plugin"); err != nil {
		t.Fatalf("expected valid id: %v", err)
	}
}

func TestSafePluginInstallDirStaysUnderRoot(t *testing.T) {
	dataRoot := t.TempDir()
	dest, err := infraplugin.SafePluginInstallDir(dataRoot, "com.example.plugin")
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := filepath.Join(dataRoot, "plugins")
	if !filepath.IsAbs(dest) {
		t.Fatalf("expected absolute path, got %q", dest)
	}
	rel, err := filepath.Rel(pluginsRoot, dest)
	if err != nil || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		t.Fatalf("dest escapes plugins root: %q", dest)
	}
}

func TestManifestValidateRejectsBadID(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "..",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid manifest")
	}
}

func TestManifestValidateRejectsEngineArgs(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.args",
		Name:    "x",
		Version: "1",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "p.exe",
			Args:  []string{"--evil"},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected engine.args rejection")
	}
}

func TestManifestValidateRejectsFSRootEscape(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "plugins", "com.example.fs")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := domainplugin.Manifest{
		ID:      "com.example.fs",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			FS: &domainplugin.FSCaps{Read: []string{"${pluginData}/../../outside"}},
		},
	}
	if err := bundle.ValidateCapabilitiesForInstall(&m, installDir); err == nil {
		t.Fatal("expected FS root escape rejection")
	}
}

func TestManifestValidateRejectsBroadEventSubscribe(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{Subscribe: []string{"core.*"}},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected broad subscribe rejection")
	}
}

func TestManifestValidateRejectsCrossPluginEventSubscribe(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Subscribe: []string{"plugin.com.other.plugin.*"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected cross-plugin subscribe rejection")
	}
}

func TestManifestValidateRejectsCrossPluginEventPublish(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Publish: []string{"plugin.com.other.plugin.*"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected cross-plugin publish rejection")
	}
}

func TestManifestValidateRejectsCoreEventPublish(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Publish: []string{"core.session.opened"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected core publish rejection")
	}
}

func TestManifestValidateRejectsBroadCoreEventPublish(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.events",
		Name:    "x",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Events: &domainplugin.EventCaps{
				Publish: []string{"core.*"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected broad core publish rejection")
	}
}
