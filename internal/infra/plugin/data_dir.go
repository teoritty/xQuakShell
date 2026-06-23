package plugin

import (
	"os"
	"path/filepath"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginInstanceDataDir returns the writable data directory for a plugin process instance.
// For per-session isolation the path is scoped under the plugin data root by session ID.
func PluginInstanceDataDir(dataRoot, pluginID, sessionID string, isolation domainplugin.IsolationMode) string {
	base := PluginDataDir(dataRoot, pluginID)
	if isolation == domainplugin.IsolationPerSession && strings.TrimSpace(sessionID) != "" {
		return filepath.Join(base, sanitizeSessionSegment(sessionID))
	}
	return base
}

func sanitizeSessionSegment(sessionID string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(sessionID) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "session"
	}
	return b.String()
}

// EnsurePluginInstanceDataDir creates the instance data directory with restrictive permissions.
func EnsurePluginInstanceDataDir(dataRoot, pluginID, sessionID string, isolation domainplugin.IsolationMode) (string, error) {
	dir := PluginInstanceDataDir(dataRoot, pluginID, sessionID, isolation)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
