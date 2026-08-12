package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginInstanceDataDir returns the writable data directory for a plugin process instance.
// For per-session isolation the path is scoped under the plugin data root by session ID.
func PluginInstanceDataDir(dataRoot, pluginID, sessionID string, isolation domainplugin.IsolationMode) string {
	base := PluginDataDir(dataRoot, pluginID)
	if scope := instanceSessionScope(sessionID, isolation); scope != "" {
		return filepath.Join(base, sanitizeSessionSegment(scope))
	}
	return base
}

// instanceSessionScope reports the session that scopes one plugin process, or "" when the process
// is scoped to the plugin alone.
//
// Everything that must be per-instance asks this one question, and that is the point rather than
// tidiness. On Windows a plugin instance also gets an AppContainer whose SID carries the ACEs on
// that data directory; if the two disagreed about whether a session scopes this process, one
// session's container SID would end up holding an ACE on another session's directory — and ADR-003
// exists precisely so those two sessions cannot see each other's files.
func instanceSessionScope(sessionID string, isolation domainplugin.IsolationMode) string {
	if isolation != domainplugin.IsolationPerSession {
		return ""
	}
	return strings.TrimSpace(sessionID)
}

// PluginInstanceKey names one plugin process instance for anything that has to be scoped to exactly
// what PluginInstanceDataDir is scoped to.
//
// The parts are length-prefixed rather than joined by a separator. A separator has to be a byte
// that cannot appear in either part, and neither a plugin id (written by a plugin author) nor a
// session id offers that guarantee for free; length prefixes need no such assumption, so no pair of
// distinct identities can render to the same key. That matters because the key is hashed into an
// AppContainer name, and two instances sharing one share every ACE granted to it.
func PluginInstanceKey(pluginID, sessionID string, isolation domainplugin.IsolationMode) string {
	scope := instanceSessionScope(sessionID, isolation)
	return fmt.Sprintf("%d:%s%d:%s", len(pluginID), pluginID, len(scope), scope)
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
