package plugin

import (
	"fmt"
	"strings"
)

// MayPublishToEventChannel reports whether pluginID may publish on channel at runtime.
// Plugins may publish only within plugin.<ownId>.*; core channels are host-only.
func MayPublishToEventChannel(pluginID, channel string) bool {
	channel = strings.TrimSpace(channel)
	if channel == "" || strings.HasPrefix(channel, "core.") {
		return false
	}
	if !strings.HasPrefix(channel, "plugin.") {
		return false
	}
	return ownsPluginNamespace(pluginID, channel)
}

// OwnsPluginEventChannel reports whether channel is within pluginID's subscribe namespace.
// Core subscribe channels return true — manifest validation enforces the core allowlist.
// Wildcard patterns (plugin.<id>.*) are accepted when the prefix matches the plugin id.
func OwnsPluginEventChannel(pluginID, channel string) bool {
	return ownsPluginNamespace(pluginID, channel)
}

func ownsPluginNamespace(pluginID, channel string) bool {
	pluginID = strings.TrimSpace(pluginID)
	channel = strings.TrimSpace(channel)
	if pluginID == "" || channel == "" {
		return false
	}
	if !strings.HasPrefix(channel, "plugin.") {
		return true // core.* subscribe channels; publish uses MayPublishToEventChannel
	}
	if strings.HasSuffix(channel, "*") {
		prefix := strings.TrimSuffix(channel, "*")
		return prefix == "plugin."+pluginID || strings.HasPrefix(prefix, "plugin."+pluginID+".")
	}
	if channel == "plugin."+pluginID {
		return true
	}
	return strings.HasPrefix(channel, "plugin."+pluginID+".")
}

// ValidatePluginEventPattern rejects manifest event patterns outside the plugin namespace.
func ValidatePluginEventPattern(pluginID, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ErrInvalidManifest
	}
	if !strings.HasPrefix(pattern, "plugin.") {
		return nil
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if prefix == "plugin."+pluginID || strings.HasPrefix(prefix, "plugin."+pluginID+".") {
			return nil
		}
		return fmt.Errorf("%w: event pattern %q outside plugin namespace", ErrInvalidManifest, pattern)
	}
	if !OwnsPluginEventChannel(pluginID, pattern) {
		return fmt.Errorf("%w: event pattern %q outside plugin namespace", ErrInvalidManifest, pattern)
	}
	return nil
}
