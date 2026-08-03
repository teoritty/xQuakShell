package capability

import (
	"net"
	"strconv"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
)

// shouldAllowResolvedIP decides whether a resolved ip may be dialed. Shared verbatim by
// every dial-policy caller (NetProxy, ChannelRelayBackend) so the authorization logic never
// drifts between callers (ADR-011 §Security). Implements "either mode permits":
//   - arbitrary mode: public IPs allowed; private/loopback only if allowPrivateNetworks
//   - allowlist mode: delegates to domain AllowResolvedDialIP unchanged
//   - if arbitrary mode doesn't allow, falls through to the allowlist check
func shouldAllowResolvedIP(allowArbitrary, allowPrivateNetworks bool, patternHost string, ip net.IP) bool {
	if allowArbitrary {
		if !domainplugin.IsRestrictedDialIP(ip) {
			return true
		}
		if allowPrivateNetworks {
			return true
		}
	}
	return domainplugin.AllowResolvedDialIP(patternHost, ip)
}

func matchNetworkPattern(pattern, host string, port int) bool {
	parsed, err := domainplugin.ParseNetworkPattern(pattern)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Host, host) {
		return false
	}
	return matchPortSpec(parsed.PortSpec, port)
}

// matchingPatternHost returns the host literal from the first matching outbound pattern.
func matchingPatternHost(patterns []string, host string, port int) (string, bool) {
	for _, pattern := range patterns {
		parsed, err := domainplugin.ParseNetworkPattern(pattern)
		if err != nil {
			continue
		}
		if !strings.EqualFold(parsed.Host, host) {
			continue
		}
		if !matchPortSpec(parsed.PortSpec, port) {
			continue
		}
		return parsed.Host, true
	}
	return "", false
}

func matchPortSpec(spec string, port int) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.Contains(spec, "*") {
		return false
	}
	if strings.Contains(spec, "-") {
		parts := strings.SplitN(spec, "-", 2)
		minPort, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxPort, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return false
		}
		return port >= minPort && port <= maxPort
	}
	want, err := strconv.Atoi(spec)
	if err != nil {
		return false
	}
	return port == want
}
