package capability

import (
	"context"
	"encoding/json"
	"net"
	"testing"

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
