package capability

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// fakeChannelDataPath is a minimal domainplugin.ChannelDataPath test double: Recv delivers
// bytes "sent by the plugin", Send records/forwards bytes "received from the relay", and
// capacity is test-controlled to drive the credit-0 backpressure assertion.
type fakeChannelDataPath struct {
	inbound  chan []byte
	closed   chan struct{}
	closeErr sync.Once
	acks     atomic.Int64

	mu       sync.Mutex
	sent     [][]byte
	capacity chan struct{} // closed = capacity available
}

func newFakeChannelDataPath() *fakeChannelDataPath {
	return &fakeChannelDataPath{
		inbound:  make(chan []byte, 16),
		closed:   make(chan struct{}),
		capacity: closedCapacityChan(),
	}
}

func closedCapacityChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// mustChannelHandle builds a handle the way the composition root does. There is no literal
// form: a handle without a data path is not constructible, which is the point.
func mustChannelHandle(t *testing.T, id uint32, pluginID, purpose, parentSessionID, hint string, data domainplugin.ChannelDataPath) *domainplugin.ChannelHandle {
	t.Helper()
	h, err := domainplugin.NewChannelHandle(id, pluginID, purpose, parentSessionID, hint, data)
	if err != nil {
		t.Fatalf("new channel handle: %v", err)
	}
	return h
}

func (f *fakeChannelDataPath) blockCapacity() {
	f.mu.Lock()
	f.capacity = make(chan struct{})
	f.mu.Unlock()
}

func (f *fakeChannelDataPath) releaseCapacity() {
	f.mu.Lock()
	close(f.capacity)
	f.mu.Unlock()
}

func (f *fakeChannelDataPath) Send(_ context.Context, payload []byte) error {
	f.mu.Lock()
	f.sent = append(f.sent, append([]byte(nil), payload...))
	f.mu.Unlock()
	return nil
}

func (f *fakeChannelDataPath) sentFrames() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeChannelDataPath) Recv() ([]byte, bool) {
	select {
	case b, ok := <-f.inbound:
		return b, ok
	case <-f.closed:
		return nil, false
	}
}

func (f *fakeChannelDataPath) pushInbound(b []byte) {
	f.inbound <- b
}

func (f *fakeChannelDataPath) closeDown() {
	f.closeErr.Do(func() { close(f.closed) })
}

func (f *fakeChannelDataPath) Ack(_ context.Context) error {
	f.acks.Add(1)
	return nil
}

func (f *fakeChannelDataPath) ackCount() int64 { return f.acks.Load() }

func (f *fakeChannelDataPath) Close() error {
	f.closeDown()
	return nil
}

func (f *fakeChannelDataPath) WaitForCapacity(ctx context.Context) error {
	f.mu.Lock()
	c := f.capacity
	f.mu.Unlock()
	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestChannelRelayBackend_HintRejectedIdenticallyToDialPolicyFixtures(t *testing.T) {
	for _, f := range dialPolicyFixtures() {
		t.Run(f.name, func(t *testing.T) {
			backend := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
				Outbound:               f.patterns,
				AllowArbitraryOutbound: f.allowArbitrary,
				AllowPrivateNetworks:   f.allowPrivate,
			}, nil)
			backend.resolver = mapResolver{f.host: {f.resolvedIP}}

			hint := net.JoinHostPort(f.host, strconv.Itoa(f.port))
			err := backend.Authorize(domainplugin.PurposeTCPRelay, "sess-1", hint)
			denied := errors.Is(err, domainplugin.ErrCapabilityDenied)
			if denied != f.wantDenied {
				t.Fatalf("%s: denied=%v (err=%v), want denied=%v", f.name, denied, err, f.wantDenied)
			}
		})
	}
}

func TestChannelRelayBackend_LoopbackBlockedUnlessAllowPrivateNetworks(t *testing.T) {
	// A hostname resolving to loopback, not a raw IP literal: matches the existing
	// dial-policy's documented carve-out (AllowResolvedDialIP) for an *explicit* IP-literal
	// pattern/hint, which this case must not accidentally trigger.
	backend := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
	}, nil)
	backend.resolver = mapResolver{"printer.local": {net.ParseIP("127.0.0.1")}}

	if err := backend.Authorize(domainplugin.PurposeTCPRelay, "sess-1", "printer.local:9"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected loopback denied without allowPrivateNetworks, got %v", err)
	}

	backend2 := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend2.resolver = mapResolver{"printer.local": {net.ParseIP("127.0.0.1")}}
	if err := backend2.Authorize(domainplugin.PurposeTCPRelay, "sess-1", "printer.local:9"); errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected loopback allowed with allowPrivateNetworks, got %v", err)
	}
}

func TestChannelRelayBackend_AuditRecordsCanonicalTargetNotRawHint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-time.After(50 * time.Millisecond)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	rawHint := net.JoinHostPort("dial-me.example", strconv.Itoa(port))
	canonical := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	var mu sync.Mutex
	var entries []domainplugin.ChannelAuditEntry
	backend := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, func(entry domainplugin.ChannelAuditEntry) {
		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()
	})
	backend.resolver = mapResolver{"dial-me.example": {net.ParseIP("127.0.0.1")}}

	if err := backend.Authorize(domainplugin.PurposeTCPRelay, "sess-1", rawHint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	handle := mustChannelHandle(t, 1, "com.test", domainplugin.PurposeTCPRelay, "sess-1", rawHint, newFakeChannelDataPath())
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	mu.Lock()
	defer mu.Unlock()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Target != canonical {
		t.Fatalf("expected canonical target %q, got %q", canonical, entries[0].Target)
	}
	if entries[0].Target == rawHint {
		t.Fatal("audit target must not equal raw hint")
	}
}

func TestChannelRelayBackend_WireDialsAndDataFlowsBothDirections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverRecv := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		serverRecv <- append([]byte(nil), buf[:n]...)
		_, _ = conn.Write([]byte("pong"))
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	host := listener.Addr().(*net.TCPAddr).IP.String()

	backend := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend.resolver = mapResolver{host: {net.ParseIP(host)}}

	hint := net.JoinHostPort(host, strconv.Itoa(port))
	if err := backend.Authorize(domainplugin.PurposeTCPRelay, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	handle := mustChannelHandle(t, 2, "com.test", domainplugin.PurposeTCPRelay, "sess-1", hint, data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	data.pushInbound([]byte("ping"))

	select {
	case got := <-serverRecv:
		if string(got) != "ping" {
			t.Fatalf("server got %q, want ping", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plugin->relay bytes")
	}

	deadline := time.After(2 * time.Second)
	for {
		frames := data.sentFrames()
		if len(frames) > 0 {
			if string(frames[0]) != "pong" {
				t.Fatalf("relay->plugin frame = %q, want pong", frames[0])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for relay->plugin frame")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestChannelRelayBackend_CreditZeroSuspendsUpstreamReads(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	host := listener.Addr().(*net.TCPAddr).IP.String()

	backend := NewChannelRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend.resolver = mapResolver{host: {net.ParseIP(host)}}

	hint := net.JoinHostPort(host, strconv.Itoa(port))
	if err := backend.Authorize(domainplugin.PurposeTCPRelay, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	data.blockCapacity()
	handle := mustChannelHandle(t, 3, "com.test", domainplugin.PurposeTCPRelay, "sess-1", hint, data)
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for accept")
	}
	defer serverConn.Close()
	_, _ = serverConn.Write([]byte("held-back"))

	time.Sleep(150 * time.Millisecond)
	if frames := data.sentFrames(); len(frames) != 0 {
		t.Fatalf("expected no frames delivered while capacity blocked, got %d", len(frames))
	}

	data.releaseCapacity()

	deadline := time.After(2 * time.Second)
	for {
		frames := data.sentFrames()
		if len(frames) > 0 {
			if string(frames[0]) != "held-back" {
				t.Fatalf("frame = %q, want held-back", frames[0])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for frame after releasing capacity")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
