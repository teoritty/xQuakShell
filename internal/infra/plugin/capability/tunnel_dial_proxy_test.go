package capability

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

type countingTunnelInbound struct {
	dials atomic.Int32
}

func (c *countingTunnelInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	c.dials.Add(1)
	return json.Marshal(map[string]string{"tunnelId": "t1"})
}

func (c *countingTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (c *countingTunnelInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
}

func (c *countingTunnelInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, domainplugin.ErrNotImplemented
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
	proxy.ReleaseSlot()
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial after ReleaseSlot: %v", err)
	}
}

func TestTunnelLocalProxy_ReleaseSlotOnBind(t *testing.T) {
	inbound := &countingTunnelInbound{}
	var released bool
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, inbound)
	local := NewTunnelLocalProxy("p1", inbound, func() { released = true; proxy.ReleaseSlot() })
	ctx := context.Background()

	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := local.Bind(ctx, json.RawMessage(`{"localConnId":"l","tunnelId":"t"}`)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !released {
		t.Fatal("expected onBind callback")
	}
	if _, err := proxy.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial after bind should succeed after slot release: %v", err)
	}
}
