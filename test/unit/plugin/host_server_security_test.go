package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func TestHostServerPing(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate:     capability.NewGate(domainplugin.Manifest{}),
	})

	raw, rpcErr := server.HandleRequest(context.Background(), "ping", nil)
	if rpcErr != nil {
		t.Fatalf("expected ping ok, got %#v", rpcErr)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["pong"] != "ok" {
		t.Fatalf("unexpected ping result %v", out)
	}
}

func TestHostServerInvalidParamsOmitsDetail(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				FS: &domainplugin.FSCaps{Read: []string{"${pluginData}"}},
			},
		}),
		FS: mustFSProxy(t, &domainplugin.FSCaps{Read: []string{"${pluginData}"}}, t.TempDir()),
	})

	_, rpcErr := server.HandleRequest(context.Background(), "fs.read", json.RawMessage([]byte("{")))
	if rpcErr == nil || rpcErr.Code != -32602 {
		t.Fatalf("expected invalid params, got %#v", rpcErr)
	}
	if len(rpcErr.Data) > 0 {
		var data map[string]string
		_ = json.Unmarshal(rpcErr.Data, &data)
		if _, ok := data["detail"]; ok {
			t.Fatalf("invalid params must not leak detail: %v", data)
		}
	}
}

func TestHostServerLogWriteRateLimited(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate:     capability.NewGate(domainplugin.Manifest{}),
	})

	params, _ := json.Marshal(map[string]string{"level": "info", "message": "hello"})
	limit := domainplugin.MaxPluginLogLinesPerSecond
	for i := 0; i < limit; i++ {
		if _, rpcErr := server.HandleRequest(context.Background(), "log.write", params); rpcErr != nil {
			t.Fatalf("log.write %d: unexpected error %#v", i, rpcErr)
		}
	}
	_, rpcErr := server.HandleRequest(context.Background(), "log.write", params)
	if rpcErr == nil || rpcErr.Code != -32003 {
		t.Fatalf("expected rate limit -32003, got %#v", rpcErr)
	}
}

func TestHostServerNilFSProxyDoesNotPanic(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				FS: &domainplugin.FSCaps{Read: []string{"${pluginData}"}},
			},
		}),
	})

	_, rpcErr := server.HandleRequest(context.Background(), "fs.read", json.RawMessage(`{"path":"x"}`))
	if rpcErr == nil || rpcErr.Code != -32603 {
		t.Fatalf("expected request failed -32603, got %#v", rpcErr)
	}
}

func TestHostServerNilNetProxyDoesNotPanic(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Network: &domainplugin.NetworkCaps{Outbound: []string{"tcp:127.0.0.1:8080"}},
			},
		}),
	})

	_, rpcErr := server.HandleRequest(context.Background(), "net.dial", json.RawMessage(`{"host":"127.0.0.1","port":8080}`))
	if rpcErr == nil || rpcErr.Code != -32603 {
		t.Fatalf("expected request failed -32603, got %#v", rpcErr)
	}
}

type sessionNotBoundHandler struct{}

func (sessionNotBoundHandler) Handle(_ context.Context, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrSessionNotBound
}

type tunnelAlreadyExistsInbound struct{}

func (tunnelAlreadyExistsInbound) TunnelDial(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelAlreadyExists
}

func (tunnelAlreadyExistsInbound) TunnelClose(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelNotFound
}

func (tunnelAlreadyExistsInbound) TunnelLocalWrite(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelNotFound
}

func (tunnelAlreadyExistsInbound) TunnelLocalClose(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelNotFound
}

func (tunnelAlreadyExistsInbound) TunnelBind(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelNotFound
}

func TestHostServerTunnelAlreadyExistsMapsConflict(t *testing.T) {
	dialProxy := capability.NewTunnelDialProxy("test", &domainplugin.TunnelCaps{Provider: true}, tunnelAlreadyExistsInbound{})
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Tunnel: &domainplugin.TunnelCaps{Provider: true},
			},
		}),
		TunnelDial: dialProxy,
	})

	_, rpcErr := server.HandleRequest(context.Background(), "tunnel.dial", json.RawMessage(`{"ruleId":"r1","targetHost":"example.com","targetPort":80}`))
	if rpcErr == nil || rpcErr.Code != -32008 {
		t.Fatalf("expected tunnel already exists -32008, got %#v", rpcErr)
	}
}

func TestHostServerSessionNotBoundMapsCapabilityDenied(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate: capability.NewGate(domainplugin.Manifest{
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{Terminal: true},
			},
		}),
		Sessions: sessionNotBoundHandler{},
	})

	_, rpcErr := server.HandleRequest(context.Background(), "session.updateState", json.RawMessage(`{"sessionId":"s1"}`))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("expected capability denied -32001, got %#v", rpcErr)
	}
}
