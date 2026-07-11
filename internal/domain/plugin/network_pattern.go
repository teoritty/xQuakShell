package plugin

import (
	"fmt"
	"strconv"
	"strings"
)

// NetworkPattern is a validated outbound allowlist entry (tcp:host:port or udp:host:port).
type NetworkPattern struct {
	Proto    string
	Host     string
	PortSpec string
}

// ParseNetworkPattern parses and validates a manifest network outbound pattern.
// Only explicit tcp:host:port / udp:host:port (or ...:host:port-range) forms are accepted.
func ParseNetworkPattern(pattern string) (NetworkPattern, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return NetworkPattern{}, fmt.Errorf("%w: empty network pattern", ErrInvalidManifest)
	}
	proto, rest, ok := cutNetworkPatternProto(pattern)
	if !ok {
		return NetworkPattern{}, fmt.Errorf("%w: network pattern must start with tcp: or udp:", ErrInvalidManifest)
	}
	if rest == "" || strings.Contains(rest, "*") {
		return NetworkPattern{}, fmt.Errorf("%w: wildcards are not allowed in network patterns", ErrInvalidManifest)
	}
	host, portSpec, ok := strings.Cut(rest, ":")
	if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(portSpec) == "" {
		return NetworkPattern{}, fmt.Errorf("%w: network pattern must be %s:host:port", ErrInvalidManifest, proto)
	}
	host = strings.TrimSpace(host)
	portSpec = strings.TrimSpace(portSpec)
	if host == "" || portSpec == "" {
		return NetworkPattern{}, fmt.Errorf("%w: network pattern must be %s:host:port", ErrInvalidManifest, proto)
	}
	if _, err := strconv.Atoi(portSpec); err == nil && !strings.Contains(portSpec, "-") {
		// host:port form — host must not be a bare port number mistaken for host-only patterns.
		if isAllDigits(host) {
			return NetworkPattern{}, fmt.Errorf("%w: ambiguous network pattern %s:%s:%s", ErrInvalidManifest, proto, host, portSpec)
		}
	}
	if err := validatePortSpec(portSpec); err != nil {
		return NetworkPattern{}, err
	}
	return NetworkPattern{Proto: proto, Host: host, PortSpec: portSpec}, nil
}

// cutNetworkPatternProto splits the leading proto token shared by both tcp: and udp: forms
// so wildcard/port validation stays a single code path regardless of proto.
func cutNetworkPatternProto(pattern string) (proto, rest string, ok bool) {
	for _, p := range []string{"tcp", "udp"} {
		if r, found := strings.CutPrefix(pattern, p+":"); found {
			return p, r, true
		}
	}
	return "", "", false
}

func validatePortSpec(spec string) error {
	if spec == "" || strings.Contains(spec, "*") {
		return fmt.Errorf("%w: invalid port in network pattern", ErrInvalidManifest)
	}
	if strings.Contains(spec, "-") {
		parts := strings.SplitN(spec, "-", 2)
		minPort, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxPort, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || minPort < 1 || maxPort > 65535 || minPort > maxPort {
			return fmt.Errorf("%w: invalid port range in network pattern", ErrInvalidManifest)
		}
		return nil
	}
	port, err := strconv.Atoi(spec)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%w: invalid port in network pattern", ErrInvalidManifest)
	}
	return nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
