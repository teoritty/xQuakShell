package capability

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

type trackingTunnelInbound struct {
	writes atomic.Int32
}

func (t *trackingTunnelInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"tunnelId": "tn-owned"})
}

func (t *trackingTunnelInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *trackingTunnelInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	t.writes.Add(1)
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *trackingTunnelInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (t *trackingTunnelInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func TestTunnelLocalProxy_RejectUnknownLocal(t *testing.T) {
	inbound := &trackingTunnelInbound{}
	dial := NewTunnelDialProxy("p1", nil, inbound)
	local := NewTunnelLocalProxy("p1", inbound, dial)
	ctx := context.Background()

	params := json.RawMessage(`{"localConnId":"lc-foreign","dataBase64":"aGVsbG8="}`)
	_, err := local.LocalWrite(ctx, params)
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("localWrite = %v, want ErrHandleNotFound", err)
	}
	if inbound.writes.Load() != 0 {
		t.Fatal("inbound should not be called for unknown localConnId")
	}

	_, err = local.LocalClose(ctx, json.RawMessage(`{"localConnId":"lc-foreign"}`))
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("localClose = %v, want ErrHandleNotFound", err)
	}

	_, err = local.Bind(ctx, json.RawMessage(`{"localConnId":"lc-foreign","tunnelId":"tn-owned"}`))
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("bind = %v, want ErrHandleNotFound", err)
	}
}

func TestTunnelLocalProxy_BindRequiresOwnedTunnel(t *testing.T) {
	inbound := &trackingTunnelInbound{}
	dial := NewTunnelDialProxy("p1", nil, inbound)
	local := NewTunnelLocalProxy("p1", inbound, dial)
	ctx := context.Background()

	local.RegisterLocal("lc-1")
	_, err := local.Bind(ctx, json.RawMessage(`{"localConnId":"lc-1","tunnelId":"tn-foreign"}`))
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("bind = %v, want ErrHandleNotFound for foreign tunnelId", err)
	}
}

func TestTunnelLocalProxy_RegisterAndReleaseLocal(t *testing.T) {
	inbound := &trackingTunnelInbound{}
	dial := NewTunnelDialProxy("p1", nil, inbound)
	local := NewTunnelLocalProxy("p1", inbound, dial)
	ctx := context.Background()

	local.RegisterLocal("lc-1")
	if _, err := local.LocalWrite(ctx, json.RawMessage(`{"localConnId":"lc-1","dataBase64":"YQ=="}`)); err != nil {
		t.Fatalf("localWrite: %v", err)
	}
	local.ReleaseLocal("lc-1")
	_, err := local.LocalWrite(ctx, json.RawMessage(`{"localConnId":"lc-1","dataBase64":"YQ=="}`))
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("after release = %v, want ErrHandleNotFound", err)
	}
}
