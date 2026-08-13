package plugin_test

import (
	"net"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestAllowResolvedDialIPBlocksPrivateResolution(t *testing.T) {
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("127.0.0.1")) {
		t.Fatal("expected hostname resolving to loopback to be denied")
	}
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("10.0.0.5")) {
		t.Fatal("expected hostname resolving to private IP to be denied")
	}
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("169.254.169.254")) {
		t.Fatal("expected link-local metadata IP to be denied")
	}
}

func TestAllowResolvedDialIPPermitsExplicitLoopbackPattern(t *testing.T) {
	if !domainplugin.AllowResolvedDialIP("127.0.0.1", net.ParseIP("127.0.0.1")) {
		t.Fatal("expected explicit loopback allowlist entry")
	}
}

func TestAllowResolvedDialIPPermitsPublicIPs(t *testing.T) {
	if !domainplugin.AllowResolvedDialIP("example.com", net.ParseIP("93.184.216.34")) {
		t.Fatal("expected public IP to be allowed")
	}
}

// The ranges below all read as ordinary public addresses to net.IP's own predicates, which is why
// each one needs naming here: a plugin with arbitrary outbound and allowPrivateNetworks off is
// supposed to reach the internet and nothing on the user's own networks.
func TestIsRestrictedDialIPCoversTheRangesGoCallsPublic(t *testing.T) {
	for _, tc := range []struct {
		ip     string
		reason string
	}{
		{"100.64.0.1", "carrier-grade NAT: where Tailscale and ZeroTier put the user's own machines"},
		{"100.127.255.255", "the top of 100.64.0.0/10"},
		{"0.1.2.3", "0.0.0.0/8 reaches the local host on Linux and macOS"},
		{"255.255.255.255", "the IPv4 broadcast address"},
		{"64:ff9b::7f00:1", "NAT64-encoded 127.0.0.1"},
		{"64:ff9b::a00:5", "NAT64-encoded 10.0.0.5"},
		{"2002:7f00:1::", "6to4-encoded 127.0.0.1"},
		{"2002::1", "6to4 wrapping 0.0.0.0"},
		{"::ffff:127.0.0.1", "IPv4-mapped loopback"},
		{"::ffff:10.0.0.5", "IPv4-mapped private address"},
		{"fd00:ec2::254", "the IPv6 cloud metadata endpoint"},
	} {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("test data %q does not parse as an IP", tc.ip)
		}
		if !domainplugin.IsRestrictedDialIP(ip) {
			t.Errorf("%s is not restricted; %s", tc.ip, tc.reason)
		}
		if domainplugin.AllowResolvedDialIP("evil.example", ip) {
			t.Errorf("a hostname resolving to %s was allowed; %s", tc.ip, tc.reason)
		}
	}
}

// The neighbours of every range added above. A restriction that swallows the public internet is a
// broken capability, not a safe default.
func TestIsRestrictedDialIPLeavesPublicAddressesAlone(t *testing.T) {
	for _, addr := range []string{
		"100.63.255.255", // just below the CGNAT block
		"100.128.0.1",    // just above it
		"1.2.3.4",
		"93.184.216.34",
		"2606:4700:4700::1111", // a public IPv6 resolver
		"2002:5db8:d822::1",    // 6to4 wrapping the public 93.184.216.34
		// NAT64 wrapping the same public address. This one pins the offset the low 32 bits are read
		// from: read them one byte early and the result is 0.93.184.216, which 0.0.0.0/8 would
		// restrict - a wrong offset that only ever errs towards refusing, and so invisible to a
		// test that checks restricted addresses alone.
		"64:ff9b::5db8:d822",
		"254.255.255.255",
	} {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("test data %q does not parse as an IP", addr)
		}
		if domainplugin.IsRestrictedDialIP(ip) {
			t.Errorf("%s is treated as restricted; it is a public address", addr)
		}
	}
}

// The allowlist carve-out is consent the user gave at install time by naming an IP literal, and it
// has to keep working for the newly restricted ranges too — a plugin whose manifest says
// tcp:100.64.0.1:8080 was granted exactly that.
func TestAnExplicitLiteralStillAllowsANewlyRestrictedRange(t *testing.T) {
	if !domainplugin.AllowResolvedDialIP("100.64.0.1", net.ParseIP("100.64.0.1")) {
		t.Error("a manifest naming 100.64.0.1 outright was refused its own address")
	}
}
