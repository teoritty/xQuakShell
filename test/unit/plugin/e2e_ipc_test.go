package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func TestPluginIPC_PingAndVaultDeny(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "com.test.echo",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{
					ReadConnectionFields: []string{"host"},
				},
			},
		}),
		Vault: capability.NewVaultProxy(nil),
	})

	raw, rpcErr := server.HandleRequest(context.Background(), "ping", nil)
	if rpcErr != nil {
		t.Fatalf("ping failed: %#v", rpcErr)
	}
	var pong map[string]string
	if err := json.Unmarshal(raw, &pong); err != nil {
		t.Fatal(err)
	}
	if pong["pong"] != "ok" {
		t.Fatalf("unexpected ping response: %v", pong)
	}

	_, rpcErr = server.HandleRequest(context.Background(), "vault.getConnection", mustJSON(map[string]string{
		"connectionId": "conn-1",
	}))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("expected vault deny without inbound port, got %#v", rpcErr)
	}
}

func TestNetProxyHandleOwnership(t *testing.T) {
	proxy := capability.NewNetProxy("plugin-a", &domainplugin.NetworkCaps{
		Outbound: []string{"tcp:127.0.0.1:1"},
	})

	_, err := proxy.Close(mustJSON(map[string]string{"handleId": "missing"}))
	if err != domainplugin.ErrHandleNotFound {
		t.Fatalf("expected ErrHandleNotFound, got %v", err)
	}
	_, err = proxy.Read(mustJSON(map[string]string{"handleId": "missing"}))
	if err != domainplugin.ErrHandleNotFound {
		t.Fatalf("expected ErrHandleNotFound, got %v", err)
	}
}
