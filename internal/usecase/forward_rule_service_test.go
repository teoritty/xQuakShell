package usecase

import (
	"context"
	"net"
	"strconv"
	"testing"

	"ssh-client/internal/domain"
)

type stubTunnelDialer struct {
	direct func(addr string) (net.Conn, error)
	listen func(addr string) (net.Listener, error)
}

func (s stubTunnelDialer) OpenDirectTCP(_ context.Context, addr string) (net.Conn, error) {
	if s.direct != nil {
		return s.direct(addr)
	}
	return nil, net.ErrClosed
}

func (s stubTunnelDialer) ListenTCP(_ context.Context, addr string) (net.Listener, error) {
	if s.listen != nil {
		return s.listen(addr)
	}
	return nil, net.ErrClosed
}

func TestForwardRuleRunner_RejectsDynamicKind(t *testing.T) {
	r := NewForwardRuleRunner(stubTunnelDialer{})
	err := r.Start(context.Background(), domain.ForwardRule{
		ID: "dyn-1", Kind: domain.ForwardRuleDynamic, BindPort: 1080,
		PluginID: "p", ProviderID: "socks5",
	})
	if err == nil {
		t.Fatal("expected error for dynamic kind")
	}
}

func TestForwardRuleRunner_StopAllOnEmpty(t *testing.T) {
	r := NewForwardRuleRunner(stubTunnelDialer{})
	r.StopAll()
	if len(r.active) != 0 {
		t.Fatalf("expected no active rules, got %d", len(r.active))
	}
}

func TestForwardRuleRunner_StartLocalBindsListener(t *testing.T) {
	r := NewForwardRuleRunner(stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return net.Dial("tcp", addr)
		},
	})
	rule := domain.ForwardRule{
		ID: "local-1", Kind: domain.ForwardRuleLocal,
		BindAddress: "127.0.0.1", BindPort: 0,
		TargetHost: "127.0.0.1", TargetPort: 9,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	rule.BindPort = mustAtoi(t, portStr)
	ln.Close()

	if err := r.Start(context.Background(), rule); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.StopAll()

	client, err := net.Dial("tcp", net.JoinHostPort(rule.BindAddress, portStr))
	if err != nil {
		t.Fatalf("dial local forward: %v", err)
	}
	client.Close()
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
