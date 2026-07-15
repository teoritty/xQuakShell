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
		Gate: newGate(t, domainplugin.Manifest{
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
	_, err = proxy.Read(context.Background(), mustJSON(map[string]string{"handleId": "missing"}))
	if err != domainplugin.ErrHandleNotFound {
		t.Fatalf("expected ErrHandleNotFound, got %v", err)
	}
}

func TestTunnelProxyHandleOwnership(t *testing.T) {
	tunnelManifest := domainplugin.Manifest{
		ID: "plugin-a",
		Capabilities: domainplugin.CapabilitySet{
			Tunnel: &domainplugin.TunnelCaps{Provider: true},
		},
	}
	inbound := &tunnelOwnershipInbound{}
	tunnelDial := capability.NewTunnelDialProxy("plugin-a", tunnelManifest.Capabilities.Tunnel, inbound)
	tunnelLocal := capability.NewTunnelLocalProxy("plugin-a", inbound, tunnelDial)
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID:    "plugin-a",
		Gate:        newGate(t, tunnelManifest),
		TunnelDial:  tunnelDial,
		TunnelLocal: tunnelLocal,
	})
	ctx := context.Background()

	_, rpcErr := server.HandleRequest(ctx, "tunnel.close", mustJSON(map[string]string{"tunnelId": "missing"}))
	if rpcErr == nil || rpcErr.Code != -32002 {
		t.Fatalf("expected resource not found for unknown tunnelId, got %#v", rpcErr)
	}
	if inbound.closes != 0 {
		t.Fatal("inbound TunnelClose should not run for unknown tunnelId")
	}

	_, rpcErr = server.HandleRequest(ctx, "tunnel.localWrite", mustJSON(map[string]string{
		"localConnId": "missing",
		"dataBase64":  "YQ==",
	}))
	if rpcErr == nil || rpcErr.Code != -32002 {
		t.Fatalf("expected resource not found for unknown localConnId, got %#v", rpcErr)
	}
	if inbound.writes != 0 {
		t.Fatal("inbound TunnelLocalWrite should not run for unknown localConnId")
	}
}

type tunnelOwnershipInbound struct {
	closes int
	writes int
}

func (t *tunnelOwnershipInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"tunnelId": "t1"})
}

func (t *tunnelOwnershipInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	t.closes++
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *tunnelOwnershipInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	t.writes++
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *tunnelOwnershipInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *tunnelOwnershipInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}
