package usecase

import (
	"context"
	"net"
	"strconv"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestTunnelDynamicService_BindRequiresBothSides(t *testing.T) {
	svc := NewTunnelDynamicService(nil, nil)
	if err := svc.Bind("missing-local", "missing-tunnel"); err != domainplugin.ErrTunnelNotFound {
		t.Fatalf("Bind() = %v, want ErrTunnelNotFound", err)
	}
}

func TestTunnelDynamicService_BindSplicesAndClearsEntries(t *testing.T) {
	local, remote := net.Pipe()
	t.Cleanup(func() { local.Close(); remote.Close() })

	svc := NewTunnelDynamicService(nil, nil)
	entry := &localEntry{
		conn: local,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	close(entry.done)
	svc.mu.Lock()
	svc.local["lc-1"] = entry
	svc.channels["tn-1"] = remote
	svc.mu.Unlock()

	if err := svc.Bind("lc-1", "tn-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if svc.HasLocal("lc-1") || svc.HasChannel("tn-1") {
		t.Fatal("entries should be removed after bind")
	}
}

func TestTunnelDynamicService_DoubleBindFails(t *testing.T) {
	local, remote := net.Pipe()
	t.Cleanup(func() { local.Close(); remote.Close() })

	svc := NewTunnelDynamicService(nil, nil)
	entry := &localEntry{
		conn: local,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	close(entry.done)
	svc.mu.Lock()
	svc.local["lc-1"] = entry
	svc.channels["tn-1"] = remote
	svc.mu.Unlock()

	if err := svc.Bind("lc-1", "tn-1"); err != nil {
		t.Fatalf("first Bind: %v", err)
	}
	if err := svc.Bind("lc-1", "tn-1"); err != domainplugin.ErrTunnelNotFound {
		t.Fatalf("second Bind() = %v, want ErrTunnelNotFound", err)
	}
}

func TestTunnelDynamicService_RegisterLocalRateLimit(t *testing.T) {
	svc := NewTunnelDynamicService(nil, nil)
	for i := 0; i < maxPreBindTunnelEntries; i++ {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		id := "lc-" + strconv.Itoa(i)
		if err := svc.RegisterLocal(context.Background(), "p", "r", id, c1); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	extra, _ := net.Pipe()
	err := svc.RegisterLocal(context.Background(), "p", "r", "overflow", extra)
	if err != domainplugin.ErrRateLimited {
		t.Fatalf("RegisterLocal overflow = %v, want ErrRateLimited", err)
	}
}
