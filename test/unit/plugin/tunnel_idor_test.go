package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type stubTunnelDialer struct {
	direct func(addr string) (net.Conn, error)
}

func (s stubTunnelDialer) OpenDirectTCP(_ context.Context, addr string) (net.Conn, error) {
	if s.direct != nil {
		return s.direct(addr)
	}
	return nil, net.ErrClosed
}

func (s stubTunnelDialer) ListenTCP(_ context.Context, _ string) (net.Listener, error) {
	return nil, net.ErrClosed
}

func startPluginATunnelSession(t *testing.T, coord *usecase.DynamicForwardCoordinator) (sessionID, ruleID string) {
	t.Helper()
	sessionID = "sess-tunnel-a"
	ruleID = "rule-tunnel-a"
	rule := domain.ForwardRule{
		ID:         ruleID,
		Kind:       domain.ForwardRuleDynamic,
		BindPort:   19080,
		PluginID:   "plugin-a",
		ProviderID: "socks5",
		Enabled:    true,
	}
	coord.StartDynamicForwardSessionForTest(context.Background(), sessionID, stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return net.Dial("tcp", addr)
		},
	}, []domain.ForwardRule{rule})
	return sessionID, ruleID
}

func TestCrossPluginTunnelDialIDOR(t *testing.T) {
	coord := usecase.NewDynamicForwardCoordinator(nil, nil)
	_, ruleID := startPluginATunnelSession(t, coord)

	params, _ := json.Marshal(map[string]any{
		"ruleId":     ruleID,
		"targetHost": "127.0.0.1",
		"targetPort": 9,
	})
	_, err := coord.TunnelDial(context.Background(), "plugin-b", params)
	if !errors.Is(err, domainplugin.ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestCrossPluginTunnelLocalWriteIDOR(t *testing.T) {
	coord := usecase.NewDynamicForwardCoordinator(nil, nil)
	sessionID, _ := startPluginATunnelSession(t, coord)

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	const localConnID = "lc-idor-test"
	if err := coord.RegisterPreBindLocalForTest(context.Background(), sessionID, "plugin-a", "rule-tunnel-a", "socks5", localConnID, local); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{
		"localConnId": localConnID,
		"dataBase64":  "aGVsbG8=",
	})
	_, err := coord.TunnelLocalWrite(context.Background(), "plugin-b", params)
	if !errors.Is(err, domainplugin.ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestCrossPluginTunnelBindIDOR(t *testing.T) {
	coord := usecase.NewDynamicForwardCoordinator(nil, nil)
	sessionID, _ := startPluginATunnelSession(t, coord)

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	const localConnID = "lc-bind-idor"
	if err := coord.RegisterPreBindLocalForTest(context.Background(), sessionID, "plugin-a", "rule-tunnel-a", "socks5", localConnID, local); err != nil {
		t.Fatal(err)
	}
	coord.RegisterTunnelOwnerForTest(sessionID, "plugin-a", "tn-bind-idor")

	params, _ := json.Marshal(map[string]string{
		"localConnId": localConnID,
		"tunnelId":    "tn-bind-idor",
	})
	_, err := coord.TunnelBind(context.Background(), "plugin-b", params)
	if !errors.Is(err, domainplugin.ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestCrossPluginTunnelCloseIDOR(t *testing.T) {
	coord := usecase.NewDynamicForwardCoordinator(nil, nil)
	sessionID, _ := startPluginATunnelSession(t, coord)
	coord.RegisterTunnelOwnerForTest(sessionID, "plugin-a", "tn-close-idor")

	params, _ := json.Marshal(map[string]string{"tunnelId": "tn-close-idor"})
	_, err := coord.TunnelClose(context.Background(), "plugin-b", params)
	if !errors.Is(err, domainplugin.ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}
