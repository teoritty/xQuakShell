package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func TestNetDialDeniedForDisallowedHost(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.net",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Network: &domainplugin.NetworkCaps{
					Outbound: []string{"tcp:127.0.0.1:9"},
				},
			},
		}),
		Net: capability.NewNetProxy("com.test.net", &domainplugin.NetworkCaps{
			Outbound: []string{"tcp:127.0.0.1:9"},
		}),
	})

	_, rpcErr := server.HandleRequest(context.Background(), "net.dial", mustJSON(map[string]any{
		"network": "tcp",
		"host":    "example.com",
		"port":    80,
	}))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("expected capability denied for foreign host, got %#v", rpcErr)
	}
}

func TestNetDialAllowedHost(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.net",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Network: &domainplugin.NetworkCaps{
					Outbound: []string{"tcp:127.0.0.1:9"},
				},
			},
		}),
		Net: capability.NewNetProxy("com.test.net", &domainplugin.NetworkCaps{
			Outbound: []string{"tcp:127.0.0.1:9"},
		}),
	})

	raw, rpcErr := server.HandleRequest(context.Background(), "net.dial", mustJSON(map[string]any{
		"network": "tcp",
		"host":    "127.0.0.1",
		"port":    9,
	}))
	if rpcErr != nil {
		if rpcErr.Code == -32603 {
			t.Skip("discard port unreachable in this environment")
		}
		t.Fatalf("expected allowed dial attempt, got %#v", rpcErr)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["handleId"] == "" {
		t.Fatalf("expected handle id, got %v", out)
	}
}
