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
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
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
