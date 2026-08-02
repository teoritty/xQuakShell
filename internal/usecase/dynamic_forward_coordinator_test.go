package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
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

func TestDynamicForwardCoordinator_PreBindLocalTimeoutReleasesOwnerAndNotifies(t *testing.T) {
	var closeNotified int
	var notifiedConnID string
	const localConnID = "lc-coord-timeout"
	notify := func(_ context.Context, pluginID, _, method string, params []byte) error {
		if method == "tunnel.localClose" {
			closeNotified++
			var p struct {
				LocalConnID string `json:"localConnId"`
			}
			_ = json.Unmarshal(params, &p)
			notifiedConnID = p.LocalConnID
			if pluginID != "plugin-a" {
				t.Errorf("pluginID = %q, want plugin-a", pluginID)
			}
		}
		return nil
	}

	coord := NewDynamicForwardCoordinator(notify, nil)
	const sessionID = "sess-prebind-timeout"
	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{}, nil)

	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	coord.mu.Unlock()
	if sf == nil || sf.service == nil {
		t.Fatal("session service not started")
	}
	sf.service.SetPreBindTimeoutForTest(50 * time.Millisecond)

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	if err := coord.RegisterPreBindLocalForTest(ctx, sessionID, "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterPreBindLocalForTest: %v", err)
	}

	coord.mu.Lock()
	_, hasOwner := coord.localOwners[localConnID]
	coord.mu.Unlock()
	if !hasOwner {
		t.Fatal("expected localOwners entry before timeout")
	}

	// evictPreBindLocal clears s.local (HasLocal) and only afterwards, via the
	// onPreBindEvict hook, clears coord.localOwners. These are two separately
	// locked pieces of state with no shared barrier, so an observer must wait
	// for both together rather than treating "HasLocal is false" as proof
	// that localOwners has already been released.
	deadline := time.Now().Add(2 * time.Second)
	for {
		coord.mu.Lock()
		_, hasOwner = coord.localOwners[localConnID]
		coord.mu.Unlock()
		if !sf.service.HasLocal(localConnID) && !hasOwner {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("entry still present after pre-bind timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if closeNotified != 1 {
		t.Fatalf("tunnel.localClose notifications = %d, want 1", closeNotified)
	}
	if notifiedConnID != localConnID {
		t.Fatalf("notified localConnId = %q, want %q", notifiedConnID, localConnID)
	}
}

func TestDynamicForwardCoordinator_PreBindLocalTimeoutReleasesLocalProxy(t *testing.T) {
	const localConnID = "lc-proxy-timeout"
	coord := NewDynamicForwardCoordinator(nil, nil)
	localProxy := capability.NewTunnelLocalProxy("plugin-a", stubTunnelLocalInbound{}, nil)

	notify := func(_ context.Context, _, _, method string, params []byte) error {
		switch method {
		case "tunnel.localAccept":
			var p struct {
				LocalConnID string `json:"localConnId"`
			}
			_ = json.Unmarshal(params, &p)
			localProxy.RegisterLocal(p.LocalConnID)
		case "tunnel.localClose":
			var p struct {
				LocalConnID string `json:"localConnId"`
			}
			_ = json.Unmarshal(params, &p)
			localProxy.ReleaseLocal(p.LocalConnID)
		}
		return nil
	}

	coord.SetNotifier(notify)
	const sessionID = "sess-proxy-timeout"
	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{}, nil)

	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	coord.mu.Unlock()
	if sf == nil || sf.service == nil {
		t.Fatal("session service not started")
	}
	sf.service.SetPreBindTimeoutForTest(50 * time.Millisecond)

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	if err := coord.RegisterPreBindLocalForTest(ctx, sessionID, "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterPreBindLocalForTest: %v", err)
	}
	if err := localRequireLocal(localProxy, localConnID); err != nil {
		t.Fatalf("expected local registered after accept: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for sf.service.HasLocal(localConnID) {
		if time.Now().After(deadline) {
			t.Fatal("entry still present after pre-bind timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The evict path removes the entry from the service map first and sends the
	// tunnel.localClose notification after (post-unlock, past stopReading and conn.Close), so
	// observing HasLocal == false does not yet imply the proxy release has happened — assert the
	// released state with the same deadline instead of instantly.
	deadline = time.Now().Add(2 * time.Second)
	for {
		err := localRequireLocal(localProxy, localConnID)
		if errors.Is(err, domainplugin.ErrHandleNotFound) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("local write after timeout = %v, want ErrHandleNotFound", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func localRequireLocal(p *capability.TunnelLocalProxy, id string) error {
	_, err := p.LocalWrite(context.Background(), json.RawMessage(`{"localConnId":"`+id+`","dataBase64":"YQ=="}`))
	return err
}

type stubTunnelLocalInbound struct{}

func (stubTunnelLocalInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrCapabilityDenied
}

func (stubTunnelLocalInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrCapabilityDenied
}

func (stubTunnelLocalInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (stubTunnelLocalInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (stubTunnelLocalInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
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
