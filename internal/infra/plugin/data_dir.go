package plugin

import (
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
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

// PluginInstanceTempDir returns the temp directory a plugin process is given as TEMP/TMP.
//
// It lives inside the instance data directory rather than beside it, so that everything a plugin
// process is permitted to write sits under the one root the host already hands it at initialize and
// already exposes as `${pluginData}`. That single root is what an OS-level sandbox has to grant:
// a temp directory outside it would have to be granted separately, which is how a sandbox acquires
// the extra permission that makes it stop being one.
func PluginInstanceTempDir(instanceDataDir string) string {
	return filepath.Join(instanceDataDir, "tmp")
}

// EnsurePluginInstanceTempDir creates the instance temp directory with restrictive permissions.
func EnsurePluginInstanceTempDir(instanceDataDir string) (string, error) {
	dir := PluginInstanceTempDir(instanceDataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
