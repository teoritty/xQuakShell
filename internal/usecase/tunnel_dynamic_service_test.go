package usecase

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"
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
	if err := svc.RegisterLocal(context.Background(), "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	t.Cleanup(func() { _ = svc.CloseLocal(localConnID) })
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.Bind(localConnID, "tn-1", nil)
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
	if err := svc.RegisterLocal(context.Background(), "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	t.Cleanup(func() { _ = svc.CloseLocal(localConnID) })
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := svc.Bind(localConnID, "tn-1", nil); err != nil {
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
	if err := svc.Bind("missing-local", "missing-tunnel", nil); err != domainplugin.ErrTunnelNotFound {
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

	if err := svc.Bind("lc-1", "tn-1", nil); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if svc.HasLocal("lc-1") || svc.HasChannel("tn-1") {
		t.Fatal("entries should be removed after bind")
	}
}

func TestTunnelDynamicService_BindInvokesOnSpliceDoneWhenConnectionEnds(t *testing.T) {
	local, peer := net.Pipe()
	tunnelLocal, tunnelRemote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		peer.Close()
		tunnelLocal.Close()
		tunnelRemote.Close()
	})

	var doneCount atomic.Int32
	svc := NewTunnelDynamicService(stubTunnelDialer{
		direct: func(addr string) (net.Conn, error) {
			return tunnelRemote, nil
		},
	}, nil)

	const localConnID = "lc-splice-done"
	if err := svc.RegisterLocal(context.Background(), "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := svc.Bind(localConnID, "tn-1", func() { doneCount.Add(1) }); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	_ = peer.Close()
	_ = tunnelLocal.Close()

	deadline := time.Now().Add(2 * time.Second)
	for doneCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("onSpliceDone not called after connection close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := doneCount.Load(); got != 1 {
		t.Fatalf("onSpliceDone called %d times, want 1", got)
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

	if err := svc.Bind("lc-1", "tn-1", nil); err != nil {
		t.Fatalf("first Bind: %v", err)
	}
	if err := svc.Bind("lc-1", "tn-1", nil); err != domainplugin.ErrTunnelNotFound {
		t.Fatalf("second Bind() = %v, want ErrTunnelNotFound", err)
	}
}

func TestTunnelDynamicService_RegisterLocalRateLimit(t *testing.T) {
	svc := NewTunnelDynamicService(nil, nil)
	svc.SetPreBindTimeoutForTest(24 * time.Hour)
	t.Cleanup(func() { svc.CloseAllPreBind() })
	for i := 0; i < maxPreBindTunnelEntries; i++ {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		id := "lc-" + strconv.Itoa(i)
		if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", id, c1); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	extra, _ := net.Pipe()
	err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", "overflow", extra)
	if err != domainplugin.ErrRateLimited {
		t.Fatalf("RegisterLocal overflow = %v, want ErrRateLimited", err)
	}
}

func TestTunnelDynamicService_LocalAcceptBeforeReaderFrames(t *testing.T) {
	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	acceptCh := make(chan struct{})
	var frameBeforeAccept bool
	notify := func(_ context.Context, _, _, method string, _ []byte) error {
		if method == "tunnel.localAccept" {
			close(acceptCh)
			return nil
		}
		if method == "tunnel.localFrame" {
			select {
			case <-acceptCh:
			default:
				frameBeforeAccept = true
			}
		}
		return nil
	}

	svc := NewTunnelDynamicService(nil, notify)
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, _ = peer.Write([]byte("early"))
	}()

	if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", "lc-early", local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	t.Cleanup(func() { _ = svc.CloseLocal("lc-early") })
	select {
	case <-acceptCh:
	case <-time.After(2 * time.Second):
		t.Fatal("tunnel.localAccept not sent")
	}
	time.Sleep(50 * time.Millisecond)
	if frameBeforeAccept {
		t.Fatal("tunnel.localFrame observed before tunnel.localAccept")
	}
}

func TestTunnelDynamicService_RegisterLocalPreBindTimeoutEvictsEntry(t *testing.T) {
	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	svc := NewTunnelDynamicService(nil, nil)
	svc.SetPreBindTimeoutForTest(50 * time.Millisecond)

	const localConnID = "lc-timeout"
	if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for svc.HasLocal(localConnID) {
		if time.Now().After(deadline) {
			t.Fatal("entry still present after pre-bind timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}

	buf := make([]byte, 1)
	if _, err := peer.Read(buf); err == nil {
		t.Fatal("expected peer read error after timeout close")
	}
}

func TestTunnelDynamicService_RegisterLocalPreBindTimeoutCancelledByBind(t *testing.T) {
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
	svc.SetPreBindTimeoutForTest(2 * time.Second)

	const localConnID = "lc-bind-cancel"
	if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if err := svc.Dial(context.Background(), "tn-1", "example.com", 80); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := svc.Bind(localConnID, "tn-1", nil); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if svc.HasLocal(localConnID) {
		t.Fatal("entry should stay removed after bind")
	}

	if _, err := peer.Write([]byte("hi")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := tunnelLocal.Read(buf); err != nil {
		t.Fatalf("tunnel read: %v", err)
	}
	if string(buf) != "hi" {
		t.Fatalf("tunnel read = %q, want hi", string(buf))
	}
}

func TestTunnelDynamicService_RegisterLocalPreBindTimeoutCancelledByCloseLocal(t *testing.T) {
	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	svc := NewTunnelDynamicService(nil, nil)
	svc.SetPreBindTimeoutForTest(2 * time.Second)

	const localConnID = "lc-close-cancel"
	if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if err := svc.CloseLocal(localConnID); err != nil {
		t.Fatalf("CloseLocal: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if svc.HasLocal(localConnID) {
		t.Fatal("entry should stay removed after CloseLocal")
	}

	buf := make([]byte, 1)
	if _, err := peer.Read(buf); err == nil {
		t.Fatal("expected peer read error after CloseLocal")
	}
}

func TestTunnelDynamicService_RegisterLocalPreBindTimeoutFreesRateLimitSlot(t *testing.T) {
	svc := NewTunnelDynamicService(nil, nil)
	svc.SetPreBindTimeoutForTest(50 * time.Millisecond)

	for i := 0; i < maxPreBindTunnelEntries; i++ {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		id := "lc-slot-" + strconv.Itoa(i)
		if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", id, c1); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for svc.HasLocal("lc-slot-0") {
		if time.Now().After(deadline) {
			t.Fatal("entries not evicted after pre-bind timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}

	extra, extraPeer := net.Pipe()
	t.Cleanup(func() { extra.Close(); extraPeer.Close() })
	if err := svc.RegisterLocal(context.Background(), "p", "r", "socks5", "lc-after-timeout", extra); err != nil {
		t.Fatalf("RegisterLocal after slot free: %v", err)
	}
	t.Cleanup(func() { _ = svc.CloseLocal("lc-after-timeout") })
}
