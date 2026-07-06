package usecase

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/plugin/capability"
	domainplugin "ssh-client/internal/domain/plugin"
)

func TestDynamicForwardCoordinator_StopSessionClosesPreBindLocals(t *testing.T) {
	coord := NewDynamicForwardCoordinator(nil, nil)
	const sessionID = "sess-stop-prebind"

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{}, nil)

	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	coord.mu.Unlock()
	if sf == nil || sf.service == nil {
		t.Fatal("session service not started")
	}

	sf.service.SetPreBindTimeoutForTest(time.Hour)

	const localConnID = "lc-stop-test"
	if err := sf.service.RegisterLocal(ctx, "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	coord.mu.Lock()
	coord.localOwners[localConnID] = tunnelHandleOwner{sessionID: sessionID, pluginID: "plugin-a"}
	coord.mu.Unlock()

	if !sf.service.HasLocal(localConnID) {
		t.Fatal("expected pre-bind local before StopSession")
	}

	coord.StopSession(sessionID)

	if sf.service.HasLocal(localConnID) {
		t.Fatal("pre-bind local still registered after StopSession")
	}

	buf := make([]byte, 1)
	if _, err := peer.Read(buf); err == nil {
		t.Fatal("expected peer read error after StopSession")
	}
}

func TestDynamicForwardCoordinator_DialSlotReleasedAfterBoundSplice(t *testing.T) {
	var releases atomic.Int32
	coord := NewDynamicForwardCoordinator(nil, nil)
	coord.SetDialSlotReleaser(func(_, _, _ string) {
		releases.Add(1)
	})

	local, peer := net.Pipe()
	tunnelLocal, tunnelRemote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		peer.Close()
		tunnelLocal.Close()
		tunnelRemote.Close()
	})

	const sessionID = "sess-splice-release"
	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return tunnelRemote, nil
		},
	}, nil)
	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	sf.rules["rule-1"] = domain.ForwardRule{
		ID: "rule-1", Kind: domain.ForwardRuleDynamic,
		BindAddress: "127.0.0.1", BindPort: 1080,
		PluginID: "plugin-a", ProviderID: "socks5", Enabled: true,
	}
	coord.mu.Unlock()

	const localConnID = "lc-splice"
	if err := coord.RegisterPreBindLocalForTest(ctx, sessionID, "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterPreBindLocalForTest: %v", err)
	}

	dialParams, _ := json.Marshal(map[string]any{
		"ruleId": "rule-1", "targetHost": "example.com", "targetPort": 80,
	})
	dialRaw, err := coord.TunnelDial(ctx, "plugin-a", dialParams)
	if err != nil {
		t.Fatalf("TunnelDial: %v", err)
	}
	var dialRes struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(dialRaw, &dialRes); err != nil {
		t.Fatalf("unmarshal dial: %v", err)
	}

	bindParams, _ := json.Marshal(map[string]string{
		"localConnId": localConnID,
		"tunnelId":    dialRes.TunnelID,
	})
	if _, err := coord.TunnelBind(ctx, "plugin-a", bindParams); err != nil {
		t.Fatalf("TunnelBind: %v", err)
	}
	if got := releases.Load(); got != 0 {
		t.Fatalf("expected 0 releases before splice end, got %d", got)
	}

	_ = peer.Close()
	_ = tunnelLocal.Close()

	deadline := time.Now().Add(2 * time.Second)
	for releases.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("dial slot not released after bound splice ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := releases.Load(); got != 1 {
		t.Fatalf("releases = %d, want 1", got)
	}
}

func TestDynamicForwardCoordinator_BoundChannelsCountTowardLimitUntilSplice(t *testing.T) {
	var releases atomic.Int32
	coord := NewDynamicForwardCoordinator(nil, nil)
	dialProxy := capability.NewTunnelDialProxy("plugin-a", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, coord)
	coord.SetDialSlotReleaser(func(_, _, tunnelID string) {
		releases.Add(1)
		dialProxy.ReleaseTunnel(tunnelID)
	})
	localProxy := capability.NewTunnelLocalProxy("plugin-a", coord, dialProxy)

	localConn, peer := net.Pipe()
	tunnelLocal, tunnelRemote := net.Pipe()
	t.Cleanup(func() {
		localConn.Close()
		peer.Close()
		tunnelLocal.Close()
		tunnelRemote.Close()
	})

	const sessionID = "sess-bound-limit"
	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return tunnelRemote, nil
		},
	}, nil)
	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	sf.rules["rule-1"] = domain.ForwardRule{
		ID: "rule-1", Kind: domain.ForwardRuleDynamic,
		BindAddress: "127.0.0.1", BindPort: 1080,
		PluginID: "plugin-a", ProviderID: "socks5", Enabled: true,
	}
	coord.mu.Unlock()

	const localConnID = "lc-bound-limit"
	if err := coord.RegisterPreBindLocalForTest(ctx, sessionID, "plugin-a", "rule-1", "socks5", localConnID, localConn); err != nil {
		t.Fatalf("RegisterPreBindLocalForTest: %v", err)
	}
	localProxy.RegisterLocal(localConnID)

	dialParams, _ := json.Marshal(map[string]any{
		"ruleId": "rule-1", "targetHost": "example.com", "targetPort": 80,
	})
	dialRaw, err := dialProxy.Dial(ctx, dialParams)
	if err != nil {
		t.Fatalf("proxy dial: %v", err)
	}
	var dialRes struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(dialRaw, &dialRes); err != nil {
		t.Fatalf("unmarshal dial: %v", err)
	}
	bindParams, _ := json.Marshal(map[string]string{
		"localConnId": localConnID,
		"tunnelId":    dialRes.TunnelID,
	})
	if _, err := localProxy.Bind(ctx, bindParams); err != nil {
		t.Fatalf("proxy bind: %v", err)
	}
	if _, err := dialProxy.Dial(ctx, dialParams); err != domainplugin.ErrRateLimited {
		t.Fatalf("second dial after bind = %v, want ErrRateLimited", err)
	}

	_ = peer.Close()
	_ = tunnelLocal.Close()

	deadline := time.Now().Add(2 * time.Second)
	for releases.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("dial slot not released after splice")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := dialProxy.Dial(ctx, dialParams); err != nil {
		t.Fatalf("dial after splice release: %v", err)
	}
}
