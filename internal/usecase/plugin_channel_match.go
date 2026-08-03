package usecase

import "strings"

// channelMatches reports whether channel matches a manifest pattern (exact or prefix*).
func channelMatches(pattern, channel string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(channel, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == channel
}

// channelAllowed reports whether channel matches any allowed pattern.
func channelAllowed(allowed []string, channel string) bool {
	for _, p := range allowed {
		if channelMatches(p, channel) {
			return true
		}
	}
	return false
}
