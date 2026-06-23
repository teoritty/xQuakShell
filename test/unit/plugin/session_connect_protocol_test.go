package plugin_test

import (
	"context"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/usecase"
)

func TestSessionBridgeRejectsUndeclaredProtocol(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: t.TempDir()})
	manager := newTestPluginManager(t, registry, host)
	bridge := usecase.NewPluginSessionBridge(manager)

	manifest := domainplugin.Manifest{
		ID:      "com.test.noproto",
		Name:    "NoProto",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{ID: "telnet", Label: "Telnet"},
			},
		},
		Capabilities: domainplugin.CapabilitySet{
			Session: &domainplugin.SessionCaps{
				ConnectProtocols: []string{"telnet"},
				Terminal:         true,
			},
		},
		Isolation: domainplugin.IsolationPerSession,
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	conn := &domain.Connection{ID: "c1", Host: "h", Port: 23, Protocol: "rdp"}
	err := bridge.Connect(context.Background(), manifest.ID, "sess-1", conn)
	if err == nil {
		t.Fatal("expected connect to be denied for undeclared protocol")
	}
}
