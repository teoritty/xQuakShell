package plugin

import (
	"net"
	"strings"
)

// IsRestrictedDialIP reports whether ip belongs to a sensitive range that plugins
// must not reach unless the manifest allowlist explicitly names that IP literal.
func IsRestrictedDialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return isRestrictedIPv4(ip4)
	}
	// An IPv6 address can carry an IPv4 one inside it, and the checks above judge the wrapper.
	// Without unwrapping, 64:ff9b::7f00:1 and 2002:7f00:1:: are simply "some public v6 address" -
	// on a network that routes either, they are 127.0.0.1.
	if embedded := embeddedIPv4(ip); embedded != nil {
		return IsRestrictedDialIP(embedded)
	}
	return false
}

// isRestrictedIPv4 covers the ranges net.IP's own predicates do not.
func isRestrictedIPv4(ip4 net.IP) bool {
	switch {
	// 100.64.0.0/10, carrier-grade NAT. Go's IsPrivate implements RFC 1918 only, so this range
	// reads as public - and it is exactly where Tailscale, ZeroTier and every ISP behind CGNAT put
	// the machines a user means when they decline to allow private networks.
	case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
		return true
	// 0.0.0.0/8. IsUnspecified only matches 0.0.0.0 itself, but the whole block is a documented
	// loopback stand-in on Linux and macOS and a standing SSRF bypass because of it.
	case ip4[0] == 0:
		return true
	// 169.254.0.0/16 is already link-local above; named again because this is the cloud metadata
	// endpoint, the single most valuable target a confined process can reach, and it must not
	// depend on a predicate someone may reorder later.
	case ip4[0] == 169 && ip4[1] == 254:
		return true
	case ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255:
		return true
	default:
		return false
	}
}

// embeddedIPv4 returns the IPv4 address carried inside an IPv6 transition address, or nil.
func embeddedIPv4(ip net.IP) net.IP {
	ip16 := ip.To16()
	if ip16 == nil {
		return nil
	}
	// NAT64 well-known prefix 64:ff9b::/96 (RFC 6052): the low 32 bits are the IPv4 address.
	if ip16[0] == 0x00 && ip16[1] == 0x64 && ip16[2] == 0xff && ip16[3] == 0x9b {
		return net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15])
	}
	// 6to4 2002::/16 (RFC 3056): the IPv4 address sits in the next 32 bits.
	if ip16[0] == 0x20 && ip16[1] == 0x02 {
		return net.IPv4(ip16[2], ip16[3], ip16[4], ip16[5])
	}
	return nil
}

// AllowResolvedDialIP reports whether resolved may be dialed for a manifest pattern host.
// Hostname patterns that resolve to restricted IPs are denied unless the pattern host
// is the same IP literal (e.g. tcp:127.0.0.1:8080).
func AllowResolvedDialIP(patternHost string, resolved net.IP) bool {
	if resolved == nil {
		return false
	}
	if !IsRestrictedDialIP(resolved) {
		return true
	}
	patternHost = strings.TrimSpace(patternHost)
	explicit := net.ParseIP(patternHost)
	return explicit != nil && explicit.Equal(resolved)
}
