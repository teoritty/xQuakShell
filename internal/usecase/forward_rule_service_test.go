package usecase

import (
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
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
	r := NewForwardRuleRunner(stubTunnelDialer{}, newStubConcurrencyLimiter(defaultForwardConnLimit))
	err := r.Start(context.Background(), domain.ForwardRule{
		ID: "dyn-1", Kind: domain.ForwardRuleDynamic, BindPort: 1080,
		PluginID: "p", ProviderID: "socks5",
	})
	if err == nil {
		t.Fatal("expected error for dynamic kind")
	}
}

func TestForwardRuleRunner_StopAllOnEmpty(t *testing.T) {
	r := NewForwardRuleRunner(stubTunnelDialer{}, newStubConcurrencyLimiter(defaultForwardConnLimit))
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
	}, newStubConcurrencyLimiter(defaultForwardConnLimit))
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

func TestForwardRuleRunner_ConcurrentStartStop(t *testing.T) {
	r := NewForwardRuleRunner(stubTunnelDialer{}, newStubConcurrencyLimiter(defaultForwardConnLimit))
	ctx := context.Background()
	rule := domain.ForwardRule{
		ID: "race-1", Kind: domain.ForwardRuleLocal,
		BindAddress: "127.0.0.1", BindPort: 0,
		TargetHost: "127.0.0.1", TargetPort: 9,
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Errorf("listen: %v", err)
				continue
			}
			_, portStr, _ := net.SplitHostPort(ln.Addr().String())
			ln.Close()
			rule.BindPort = mustAtoi(t, portStr)
			_ = r.Start(ctx, rule)
			r.Stop(rule.ID)
		}
		close(done)
	}()
	go func() {
		for i := 0; i < 20; i++ {
			r.StopAll()
		}
	}()
	<-done
}

func TestForwardRuleRunner_RemoteForwardUsesLimiter(t *testing.T) {
	counter := &countingLimiter{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	r := NewForwardRuleRunner(stubTunnelDialer{
		listen: func(addr string) (net.Listener, error) {
			return ln, nil
		},
	}, counter)

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	rule := domain.ForwardRule{
		ID: "remote-limit", Kind: domain.ForwardRuleRemote,
		BindAddress: "127.0.0.1", BindPort: mustAtoi(t, portStr),
		TargetHost: "127.0.0.1", TargetPort: 9,
	}
	if err := r.Start(context.Background(), rule); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.StopAll()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial remote forward: %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if counter.AcquireCount() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("remote forward splice did not acquire limiter")
}

type countingLimiter struct {
	mu    sync.Mutex
	count int
}

func (c *countingLimiter) Acquire(context.Context) error {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return nil
}

func (c *countingLimiter) Release() {}

func (c *countingLimiter) SetLimit(int) {}

func (c *countingLimiter) AcquireCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
