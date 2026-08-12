package plugin

import (
	"os"
	"strings"
)

// Keys copied from the host environment when required for OS/runtime compatibility.
// Secrets and user profile paths are never forwarded to plugin processes.
var allowedPluginEnvKeys = map[string]struct{}{
	"SYSTEMROOT":  {},
	"SYSTEMDRIVE": {},
	"OS":          {},
	"LANG":        {},
	"LC_ALL":      {},
	"LC_CTYPE":    {},
	"TZ":          {},
}

var blockedPluginEnvPrefixes = []string{
	"SECRET", "TOKEN", "PASSWORD", "PASSWD", "API_KEY", "APIKEY",
	"AWS_", "AZURE_", "GCP_", "GITHUB_", "GITLAB_", "NPM_",
	"VAULT_", "SSH_", "PG", "MYSQL", "REDIS_", "DATABASE_",
}

var blockedPluginEnvKeys = map[string]struct{}{
	"HOME":            {},
	"USERPROFILE":     {},
	"APPDATA":         {},
	"LOCALAPPDATA":    {},
	"TEMP":            {},
	"TMP":             {},
	"TMPDIR":          {},
	"USER":            {},
	"USERNAME":        {},
	"LOGNAME":         {},
	"COMPUTERNAME":    {},
	"HOSTNAME":        {},
	"XDG_CONFIG_HOME": {},
	"XDG_DATA_HOME":   {},
	"XDG_CACHE_HOME":  {},
	"XDG_STATE_HOME":  {},
}

// PluginProcessEnv builds a sanitized environment for an out-of-process plugin.
//
// instanceDataDir is the plugin's own data directory, not the portable data root. TEMP and TMP
// point inside it, which is what keeps the host's staging directory (<dataRoot>/tmp, where plugin
// downloads and bundles are unpacked and verified) out of reach of the processes the host is
// installing alongside. The caller creates both directories before spawning; this function only
// names them, so that the path exists in exactly one place (PluginInstanceTempDir).
func PluginProcessEnv(instanceDataDir, pluginID, sessionID string) []string {
	env := []string{
		"XQS_PLUGIN=1",
		"XQS_PLUGIN_ID=" + pluginID,
	}
	if sessionID != "" {
		env = append(env, "XQS_PLUGIN_SESSION_ID="+sessionID)
	}

	// TEMP and TMP are what Windows reads; TMPDIR is what everything else does, including Go's own
	// os.TempDir. Setting only the first two left every Unix plugin writing to /tmp — harmless
	// while nothing confined it, and a denied write the moment Landlock did, because the sandbox
	// grants the plugin's own temp directory and not the shared one.
	tempDir := PluginInstanceTempDir(instanceDataDir)
	env = append(env, "TEMP="+tempDir, "TMP="+tempDir, "TMPDIR="+tempDir)

	for _, entry := range os.Environ() {
		key, val, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if !allowHostEnvKey(key) {
			continue
		}
		env = append(env, key+"="+val)
	}
	return env
}

func allowHostEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	if _, blocked := blockedPluginEnvKeys[upper]; blocked {
		return false
	}
	for _, prefix := range blockedPluginEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return false
		}
	}
	_, allowed := allowedPluginEnvKeys[upper]
	return allowed
}
