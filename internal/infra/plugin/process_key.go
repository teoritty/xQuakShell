package plugin

import (
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

func processKey(plugin domainplugin.InstalledPlugin, sessionID string) string {
	if plugin.Manifest.EffectiveIsolation() == domainplugin.IsolationPerSession && sessionID != "" {
		return plugin.Manifest.ID + "\x00" + sessionID
	}
	return plugin.Manifest.ID
}

func pluginIDFromProcessKey(key string) string {
	if i := strings.IndexByte(key, '\x00'); i >= 0 {
		return key[:i]
	}
	return key
}
