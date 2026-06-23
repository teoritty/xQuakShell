package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

func TestRegisterRejectsProtocolConflict(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.a", Name: "A", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Contributions: domainplugin.Contributions{
				ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{{ID: "telnet", Label: "Telnet"}},
			},
		},
	})

	err := registry.Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.b", Name: "B", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "b.exe"},
			Contributions: domainplugin.Contributions{
				ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{{ID: "telnet", Label: "Telnet"}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected protocol conflict on register")
	}
}

func TestRegisterAllowsReplacingSamePluginID(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	first := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.plugin.a", Name: "A", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"},
			Contributions: domainplugin.Contributions{
				ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{{ID: "telnet", Label: "Telnet"}},
			},
		},
	}
	mustRegister(t, registry, first)

	second := first
	second.Manifest.Version = "2"
	if err := registry.Register(second); err != nil {
		t.Fatalf("expected replace of same plugin id, got %v", err)
	}
}
