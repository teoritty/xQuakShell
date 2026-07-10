package plugin

import (
	"context"
	"encoding/json"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

type tunnelNotifyInbound struct{}

func (tunnelNotifyInbound) TunnelDial(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"tunnelId": "t1"})
}

func (tunnelNotifyInbound) TunnelClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (tunnelNotifyInbound) TunnelLocalWrite(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (tunnelNotifyInbound) TunnelLocalClose(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func (tunnelNotifyInbound) TunnelBind(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

func TestProcessHost_syncTunnelLocalNotify(t *testing.T) {
	host := &ProcessHost{}
	inbound := tunnelNotifyInbound{}
	dial := capability.NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{Provider: true}, inbound)
	local := capability.NewTunnelLocalProxy("p1", inbound, dial)
	mp := &managedProcess{tunnelLocal: local}

	acceptParams, _ := json.Marshal(map[string]string{"localConnId": "lc-notify"})
	host.syncTunnelLocalNotify(mp, "tunnel.localAccept", acceptParams)

	if err := localRequireLocal(local, "lc-notify"); err != nil {
		t.Fatalf("expected registered local after accept notify: %v", err)
	}

	closeParams, _ := json.Marshal(map[string]string{"localConnId": "lc-notify"})
	host.syncTunnelLocalNotify(mp, "tunnel.localClose", closeParams)

	if err := localRequireLocal(local, "lc-notify"); err == nil {
		t.Fatal("expected local released after close notify")
	}
}

func localRequireLocal(p *capability.TunnelLocalProxy, id string) error {
	_, err := p.LocalWrite(context.Background(), json.RawMessage(`{"localConnId":"`+id+`","dataBase64":"YQ=="}`))
	return err
}

func TestReleaseTunnelDialSlot(t *testing.T) {
	host := &ProcessHost{processes: make(map[string]*managedProcess)}
	inbound := tunnelNotifyInbound{}
	dial := capability.NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 1}, inbound)
	local := capability.NewTunnelLocalProxy("p1", inbound, dial)
	key := "p1"
	host.processes[key] = &managedProcess{tunnelDial: dial, tunnelLocal: local}

	ctx := context.Background()
	if _, err := dial.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := dial.Dial(ctx, json.RawMessage(`{}`)); err != domainplugin.ErrRateLimited {
		t.Fatalf("second dial = %v, want rate limited", err)
	}

	host.ReleaseTunnelDialSlot("p1", "", "t1")

	if _, err := dial.Dial(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("dial after host release: %v", err)
	}
}
