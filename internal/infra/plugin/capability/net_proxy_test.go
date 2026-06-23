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
