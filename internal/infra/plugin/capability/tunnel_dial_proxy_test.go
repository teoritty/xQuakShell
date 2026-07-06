package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

type countingTunnelInbound struct {
	dials  atomic.Int32
	closes atomic.Int32
}

func (c *countingTunnelInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	c.dials.Add(1)
	return json.Marshal(map[string]string{"tunnelId": "t1"})
}

func (c *countingTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	c.closes.Add(1)
	return json.Marshal(map[string]bool{"ok": true})
}

func (c *countingTunnelInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (c *countingTunnelInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (c *countingTunnelInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func TestTunnelDialProxy_EnforcesChannelLimit(t *testing.T) {
	inbound := &countingTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, inbound)
	ctx := context.Background()

	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first dial: %v", err)
	}
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != domainplugin.ErrRateLimited {
		t.Fatalf("second dial = %v, want ErrRateLimited", err)
	}
	proxy.ReleaseTunnel("t1")
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial after ReleaseTunnel: %v", err)
	}
}

func TestTunnelDialProxy_RejectUnknownTunnelOnClose(t *testing.T) {
	inbound := &countingTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", nil, inbound)
	ctx := context.Background()

	_, err := proxy.Close(ctx, json.RawMessage(`{"tunnelId":"foreign"}`))
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("expected ErrHandleNotFound, got %v", err)
	}
	if inbound.closes.Load() != 0 {
		t.Fatal("inbound TunnelClose should not be called for unknown tunnelId")
	}
}

func TestTunnelDialProxy_CommitsHandleOnDial(t *testing.T) {
	inbound := &countingTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", nil, inbound)
	ctx := context.Background()

	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := proxy.requireTunnel("t1"); err != nil {
		t.Fatalf("expected owned tunnel t1: %v", err)
	}
	if _, err := proxy.Close(ctx, json.RawMessage(`{"tunnelId":"t1"}`)); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := proxy.requireTunnel("t1"); !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("expected tunnel removed after close, got %v", err)
	}
}

func TestTunnelLocalProxy_BindDoesNotReleaseChannelSlot(t *testing.T) {
	inbound := &countingTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, inbound)
	local := NewTunnelLocalProxy("p1", inbound, proxy)
	ctx := context.Background()

	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial: %v", err)
	}
	local.RegisterLocal("l")
	if _, err := local.Bind(ctx, json.RawMessage(`{"localConnId":"l","tunnelId":"t1"}`)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("dial after bind = %v, want ErrRateLimited", err)
	}
	if got := reservedTunnelSlots(proxy); got != 1 {
		t.Fatalf("expected 1 reserved slot after bind, got %d", got)
	}
}

type sequentialTunnelInbound struct {
	countingTunnelInbound
	mu   sync.Mutex
	next int
}

func (s *sequentialTunnelInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	s.next++
	id := fmt.Sprintf("t%d", s.next)
	s.mu.Unlock()
	s.dials.Add(1)
	return json.Marshal(map[string]string{"tunnelId": id})
}

func (s *sequentialTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrTunnelNotFound
}

func TestTunnelDialProxy_BypassBlockedAfterBindAll(t *testing.T) {
	const max = 2
	inbound := &sequentialTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: max}, inbound)
	local := NewTunnelLocalProxy("p1", inbound, proxy)
	ctx := context.Background()

	for i := 1; i <= max; i++ {
		if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		localID := fmt.Sprintf("l%d", i)
		local.RegisterLocal(localID)
		tunnelID := fmt.Sprintf("t%d", i)
		if _, err := local.Bind(ctx, json.RawMessage(fmt.Sprintf(`{"localConnId":%q,"tunnelId":%q}`, localID, tunnelID))); err != nil {
			t.Fatalf("bind %d: %v", i, err)
		}
	}
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("dial after binding all channels = %v, want ErrRateLimited", err)
	}
	if got := reservedTunnelSlots(proxy); got != max {
		t.Fatalf("expected %d reserved slots, got %d", max, got)
	}
}

func TestTunnelDialProxy_CloseAfterBindDoesNotReleaseSlot(t *testing.T) {
	inbound := &sequentialTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, inbound)
	local := NewTunnelLocalProxy("p1", inbound, proxy)
	ctx := context.Background()

	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial: %v", err)
	}
	local.RegisterLocal("l1")
	if _, err := local.Bind(ctx, json.RawMessage(`{"localConnId":"l1","tunnelId":"t1"}`)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := proxy.Close(ctx, json.RawMessage(`{"tunnelId":"t1"}`)); !errors.Is(err, domainplugin.ErrTunnelNotFound) {
		t.Fatalf("close after bind = %v, want ErrTunnelNotFound", err)
	}
	if got := reservedTunnelSlots(proxy); got != 1 {
		t.Fatalf("expected slot to remain after failed close, got %d reserved", got)
	}
}

type slowTunnelInbound struct {
	release chan struct{}
}

func (s *slowTunnelInbound) TunnelDial(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return json.Marshal(map[string]string{"tunnelId": "slow"})
	}
}

func (s *slowTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (s *slowTunnelInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (s *slowTunnelInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (s *slowTunnelInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func reservedTunnelSlots(p *TunnelDialProxy) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.tunnels) + p.pendingDials
}

func TestTunnelDialProxy_EnforcesChannelLimitUnderConcurrency(t *testing.T) {
	inbound := &slowTunnelInbound{release: make(chan struct{}, 8)}
	max := 2
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: max}, inbound)

	for i := 0; i < max-1; i++ {
		proxy.mu.Lock()
		proxy.tunnels["stub"] = struct{}{}
		proxy.mu.Unlock()
	}

	const workers = 16
	var wg sync.WaitGroup
	var peak atomic.Int32
	stopPeak := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopPeak:
				return
			default:
				if n := int32(reservedTunnelSlots(proxy)); n > peak.Load() {
					peak.Store(n)
				}
			}
		}
	}()

	var successes atomic.Int32
	var rateLimited atomic.Int32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := proxy.Dial(context.Background(), json.RawMessage(`{}`))
			if err == nil {
				successes.Add(1)
				return
			}
			if errors.Is(err, domainplugin.ErrRateLimited) {
				rateLimited.Add(1)
				return
			}
			t.Errorf("unexpected dial error: %v", err)
		}()
	}
	close(inbound.release)
	wg.Wait()
	close(stopPeak)

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 successful dial, got %d", got)
	}
	if got := rateLimited.Load(); got != workers-1 {
		t.Fatalf("expected %d rate-limited dials, got %d", workers-1, got)
	}
	if got := int(peak.Load()); got > max {
		t.Fatalf("peak reserved slots %d exceeds max %d", got, max)
	}
	if got := reservedTunnelSlots(proxy); got != max {
		t.Fatalf("expected %d reserved slots after test, got %d", max, got)
	}
	if proxy.pendingDials != 0 {
		t.Fatalf("expected pendingDials=0, got %d", proxy.pendingDials)
	}
}

func TestTunnelDialProxy_PendingDialsReleasedOnDialFailure(t *testing.T) {
	inbound := &countingTunnelInbound{}
	inbound.dials.Store(0)
	failInbound := &failingTunnelInbound{}
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 4}, failInbound)

	_, err := proxy.Dial(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected dial failure")
	}
	if proxy.pendingDials != 0 {
		t.Fatalf("expected pendingDials=0 after failed dial, got %d", proxy.pendingDials)
	}
	if got := reservedTunnelSlots(proxy); got != 0 {
		t.Fatalf("expected 0 reserved slots, got %d", got)
	}
}

type failingTunnelInbound struct{}

func (failingTunnelInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("dial failed")
}

func (failingTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (failingTunnelInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (failingTunnelInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (failingTunnelInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}
