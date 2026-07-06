package usecase

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestTunnelDynamicService_BindUnblocksWithoutClientData(t *testing.T) {
	local, peer := net.Pipe()
	tunnelLocal, tunnelRemote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		peer.Close()
		tunnelLocal.Close()
		tunnelRemote.Close()
	})

	svc := NewTunnelDynamicService(stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return tunnelRemote, nil
		},
	}, nil)

	const localConnID = "lc-blocked-read"
	if err := svc.RegisterLocal(context.Background(), "plugin-a", "rule-1", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.Bind(localConnID, "tn-1")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Bind: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bind blocked while reader was waiting on empty client")
	}

	if _, err := peer.Write([]byte("hello")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := tunnelLocal.Read(buf); err != nil {
		t.Fatalf("tunnel read after bind: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("tunnel read = %q, want hello", string(buf))
	}
}

func TestTunnelDynamicService_BindDoesNotNotifyLocalCloseOnStop(t *testing.T) {
	local, peer := net.Pipe()
	tunnelLocal, tunnelRemote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		peer.Close()
		tunnelLocal.Close()
		tunnelRemote.Close()
	})

	var closeNotified bool
	notify := func(_ context.Context, _, _, method string, params []byte) error {
		if method == "tunnel.localClose" {
			closeNotified = true
		}
		return nil
	}

	svc := NewTunnelDynamicService(stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return tunnelRemote, nil
		},
	}, notify)

	const localConnID = "lc-no-close-notify"
	if err := svc.RegisterLocal(context.Background(), "plugin-a", "rule-1", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := svc.Bind(localConnID, "tn-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if closeNotified {
		t.Fatal("Bind should not notify tunnel.localClose when stopping reader intentionally")
	}
}

func TestTunnelDynamicService_DialRejectsDuplicateTunnelID(t *testing.T) {
	c1, _ := net.Pipe()
	c2, _ := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	call := 0
	svc := NewTunnelDynamicService(stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			call++
			if call == 1 {
				return c1, nil
			}
			return c2, nil
		},
	}, nil)

	if err := svc.Dial(context.Background(), "tn-dup", "example.com", 80); err != nil {
		t.Fatalf("first Dial: %v", err)
	}
	if err := svc.Dial(context.Background(), "tn-dup", "example.com", 80); err != domainplugin.ErrTunnelNotFound {
		t.Fatalf("second Dial = %v, want ErrTunnelNotFound", err)
	}
}

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
