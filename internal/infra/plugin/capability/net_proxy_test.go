package capability

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

type mapResolver map[string][]net.IP

func (m mapResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := m[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	out := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

func TestNetProxyDialBlocksDNSRebindingToLoopback(t *testing.T) {
	proxy := NewNetProxy("", &domainplugin.NetworkCaps{
		Outbound: []string{"tcp:evil.example:443"},
	})
	proxy.resolver = mapResolver{
		"evil.example": {net.ParseIP("127.0.0.1")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"evil.example","port":443}`))
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected capability denied for rebinding, got %v", err)
	}
}

func TestNetProxyDialAllowsExplicitLoopbackPattern(t *testing.T) {
	proxy := NewNetProxy("", &domainplugin.NetworkCaps{
		Outbound: []string{"tcp:127.0.0.1:9"},
	})
	proxy.resolver = mapResolver{
		"127.0.0.1": {net.ParseIP("127.0.0.1")},
	}

	// Connection may fail (nothing listening) but must not be capability denied.
	_, err := proxy.Dial(json.RawMessage(`{"host":"127.0.0.1","port":9}`))
	if err == domainplugin.ErrCapabilityDenied {
		t.Fatal("expected explicit loopback allowlist to pass policy check")
	}
}

func TestNetProxyArbitraryDialPublicHost(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
	})
	proxy.resolver = mapResolver{
		"example.com": {net.ParseIP("93.184.216.34")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"example.com","port":80}`))
	if err == domainplugin.ErrCapabilityDenied {
		t.Fatal("expected arbitrary public dial to pass policy check")
	}
}

func TestNetProxyArbitraryBlocksPrivateWithoutFlag(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
	})
	proxy.resolver = mapResolver{
		"internal.local": {net.ParseIP("10.0.0.1")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"internal.local","port":23}`))
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected capability denied for private IP, got %v", err)
	}
}

func TestNetProxyArbitraryAllowsPrivateWithFlag(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	})
	proxy.resolver = mapResolver{
		"10.0.0.1": {net.ParseIP("10.0.0.1")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"10.0.0.1","port":23}`))
	if err == domainplugin.ErrCapabilityDenied {
		t.Fatal("expected private dial allowed with allowPrivateNetworks")
	}
}

func TestNetProxyCombinedAllowlistPermitsPrivateInList(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		Outbound:               []string{"tcp:192.168.1.1:23"},
		AllowArbitraryOutbound: true,
	})
	proxy.resolver = mapResolver{
		"192.168.1.1": {net.ParseIP("192.168.1.1")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"192.168.1.1","port":23}`))
	if err == domainplugin.ErrCapabilityDenied {
		t.Fatal("expected allowlist to permit private IP even when arbitrary blocks it")
	}
}

func TestNetProxyCombinedBlocksPrivateNotInList(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		Outbound:               []string{"tcp:192.168.1.1:23"},
		AllowArbitraryOutbound: true,
	})
	proxy.resolver = mapResolver{
		"10.0.0.5": {net.ParseIP("10.0.0.5")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"10.0.0.5","port":23}`))
	if err != domainplugin.ErrCapabilityDenied {
		t.Fatalf("expected capability denied when both modes block, got %v", err)
	}
}

type idleConn struct {
	unblock chan struct{}
}

func newIdleConn() *idleConn {
	return &idleConn{unblock: make(chan struct{}, 1)}
}

func (c *idleConn) Read(_ []byte) (int, error) {
	<-c.unblock
	return 0, &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
}

func (c *idleConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *idleConn) Close() error {
	close(c.unblock)
	return nil
}
func (c *idleConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *idleConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }
func (c *idleConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}
func (c *idleConn) SetReadDeadline(t time.Time) error {
	if !t.IsZero() && !t.After(time.Now()) {
		select {
		case c.unblock <- struct{}{}:
		default:
		}
	}
	return nil
}
func (c *idleConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNetProxyReadReturnsEmptyOnContextCancel(t *testing.T) {
	proxy := NewNetProxy("com.test", nil)
	conn := newIdleConn()
	proxy.mu.Lock()
	proxy.handles["h1"] = conn
	proxy.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	raw, err := proxy.Read(ctx, json.RawMessage(`{"handleId":"h1","maxBytes":64}`))
	if err != nil {
		t.Fatalf("expected empty success on idle read, got error: %v", err)
	}
	var result netReadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.EOF {
		t.Fatal("expected eof=false on idle read")
	}
	if result.ContentBase64 != "" {
		t.Fatalf("expected empty content, got %q", result.ContentBase64)
	}
}

func TestNetProxyReadReturnsDataWhenAvailable(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _ = server.Write([]byte("banner"))
	}()

	proxy := NewNetProxy("com.test", nil)
	proxy.mu.Lock()
	proxy.handles["h1"] = client
	proxy.mu.Unlock()

	raw, err := proxy.Read(context.Background(), json.RawMessage(`{"handleId":"h1","maxBytes":64}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var result netReadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.EOF {
		t.Fatal("expected eof=false while connection open")
	}
	if result.ContentBase64 == "" {
		t.Fatal("expected banner bytes")
	}
}
