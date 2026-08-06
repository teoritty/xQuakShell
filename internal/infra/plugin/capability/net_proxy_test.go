package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// dialPolicyFixture is one dial-policy decision case, shared verbatim between NetProxy's own
// tests and any other caller of the dial-policy chain (ChannelRelayBackend's tcp-relay
// backend, ADR-011) so the two never assert diverging behavior for the same inputs.
type dialPolicyFixture struct {
	name           string
	patterns       []string
	allowArbitrary bool
	allowPrivate   bool
	host           string
	resolvedIP     net.IP
	port           int
	wantDenied     bool
}

// dialPolicyFixtures is the single source of truth for dial-policy accept/deny cases. Every
// caller of the shared matchingPatternHost/shouldAllowResolvedIP chain (NetProxy.Dial,
// ChannelRelayBackend.Authorize) is expected to be exercised against this same table.
func dialPolicyFixtures() []dialPolicyFixture {
	return []dialPolicyFixture{
		{
			name:       "dns_rebinding_to_loopback_denied",
			patterns:   []string{"tcp:evil.example:443"},
			host:       "evil.example",
			resolvedIP: net.ParseIP("127.0.0.1"),
			port:       443,
			wantDenied: true,
		},
		{
			name:       "explicit_loopback_pattern_allowed",
			patterns:   []string{"tcp:127.0.0.1:9"},
			host:       "127.0.0.1",
			resolvedIP: net.ParseIP("127.0.0.1"),
			port:       9,
			wantDenied: false,
		},
		{
			name:           "arbitrary_public_host_allowed",
			allowArbitrary: true,
			host:           "example.com",
			resolvedIP:     net.ParseIP("93.184.216.34"),
			port:           80,
			wantDenied:     false,
		},
		{
			name:           "arbitrary_blocks_private_without_flag",
			allowArbitrary: true,
			host:           "internal.local",
			resolvedIP:     net.ParseIP("10.0.0.1"),
			port:           23,
			wantDenied:     true,
		},
		{
			// The case above proves the block using a *hostname*. The caller chooses that
			// spelling, and an IP literal is the spelling that used to walk straight through:
			// with no patterns configured, patternHost fell back to the caller's own host, so
			// AllowResolvedDialIP's "explicit pattern host == resolved IP" carve-out matched
			// the request against itself. The carve-out exists for an IP the *manifest*
			// allowlists — consent the user gave at install time — never for one the caller
			// names at dial time. Without this fixture, allowPrivateNetworks was decorative
			// for any arbitrary-outbound plugin.
			name:           "arbitrary_blocks_private_ip_literal_without_flag",
			allowArbitrary: true,
			host:           "10.0.0.1",
			resolvedIP:     net.ParseIP("10.0.0.1"),
			port:           23,
			wantDenied:     true,
		},
		{
			name:           "arbitrary_blocks_loopback_ip_literal_without_flag",
			allowArbitrary: true,
			host:           "127.0.0.1",
			resolvedIP:     net.ParseIP("127.0.0.1"),
			port:           5900,
			wantDenied:     true,
		},
		{
			name:           "arbitrary_allows_private_with_flag",
			allowArbitrary: true,
			allowPrivate:   true,
			host:           "10.0.0.1",
			resolvedIP:     net.ParseIP("10.0.0.1"),
			port:           23,
			wantDenied:     false,
		},
		{
			name:           "combined_allowlist_permits_private_in_list",
			patterns:       []string{"tcp:192.168.1.1:23"},
			allowArbitrary: true,
			host:           "192.168.1.1",
			resolvedIP:     net.ParseIP("192.168.1.1"),
			port:           23,
			wantDenied:     false,
		},
		{
			name:           "combined_blocks_private_not_in_list",
			patterns:       []string{"tcp:192.168.1.1:23"},
			allowArbitrary: true,
			host:           "10.0.0.5",
			resolvedIP:     net.ParseIP("10.0.0.5"),
			port:           23,
			wantDenied:     true,
		},
	}
}

func TestNetProxyDialPolicyFixtures(t *testing.T) {
	for _, f := range dialPolicyFixtures() {
		t.Run(f.name, func(t *testing.T) {
			proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
				Outbound:               f.patterns,
				AllowArbitraryOutbound: f.allowArbitrary,
				AllowPrivateNetworks:   f.allowPrivate,
			})
			proxy.resolver = mapResolver{f.host: {f.resolvedIP}}

			_, err := proxy.Dial(json.RawMessage(fmt.Sprintf(`{"host":%q,"port":%d}`, f.host, f.port)))
			denied := errors.Is(err, domainplugin.ErrCapabilityDenied)
			if denied != f.wantDenied {
				t.Fatalf("%s: denied=%v (err=%v), want denied=%v", f.name, denied, err, f.wantDenied)
			}
		})
	}
}

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

func reservedNetSlots(p *NetProxy) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.handles) + p.pendingDials
}

func TestNetProxyDialEnforcesConnectionLimitUnderConcurrency(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	host := listener.Addr().(*net.TCPAddr).IP.String()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		Outbound: []string{fmt.Sprintf("tcp:%s:%d", host, port)},
	})
	proxy.resolver = mapResolver{
		host: {net.ParseIP(host)},
	}

	for i := 0; i < domainplugin.MaxNetConnectionsPerPlugin-1; i++ {
		proxy.mu.Lock()
		proxy.handles[fmt.Sprintf("stub-%d", i)] = newIdleConn()
		proxy.mu.Unlock()
	}

	const workers = 32
	var wg sync.WaitGroup
	var peak atomic.Int32
	stopPeak := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopPeak:
				return
			default:
				if n := int32(reservedNetSlots(proxy)); n > peak.Load() {
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
			_, err := proxy.Dial(json.RawMessage(fmt.Sprintf(`{"host":%q,"port":%d}`, host, port)))
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
	wg.Wait()
	close(stopPeak)

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 successful dial, got %d", got)
	}
	if got := rateLimited.Load(); got != workers-1 {
		t.Fatalf("expected %d rate-limited dials, got %d", workers-1, got)
	}
	if got := int(peak.Load()); got > domainplugin.MaxNetConnectionsPerPlugin {
		t.Fatalf("peak reserved slots %d exceeds max %d", got, domainplugin.MaxNetConnectionsPerPlugin)
	}
	if got := reservedNetSlots(proxy); got != domainplugin.MaxNetConnectionsPerPlugin {
		t.Fatalf("expected %d reserved slots after test, got %d", domainplugin.MaxNetConnectionsPerPlugin, got)
	}
	if proxy.pendingDials != 0 {
		t.Fatalf("expected pendingDials=0, got %d", proxy.pendingDials)
	}
}

func TestNetProxyPendingDialsReleasedOnDialFailure(t *testing.T) {
	proxy := NewNetProxy("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
	})
	proxy.resolver = mapResolver{
		"127.0.0.1": {net.ParseIP("127.0.0.1")},
	}

	_, err := proxy.Dial(json.RawMessage(`{"host":"127.0.0.1","port":1}`))
	if err == nil {
		t.Fatal("expected dial failure to closed port")
	}
	if proxy.pendingDials != 0 {
		t.Fatalf("expected pendingDials=0 after failed dial, got %d", proxy.pendingDials)
	}
	if got := reservedNetSlots(proxy); got != 0 {
		t.Fatalf("expected 0 reserved slots, got %d", got)
	}
}
