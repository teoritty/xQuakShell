package capability

import (
	"strconv"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

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
