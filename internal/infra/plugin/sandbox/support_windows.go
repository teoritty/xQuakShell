//go:build windows

package sandbox

import (
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Support reports what this Windows build can enforce.
//
// An AppContainer confines both dimensions at once, which is why Windows reports whole enforcement
// where Linux cannot: the process runs under a container SID that appears in no ACL it was not
// given, and — because no capability SID is granted — it has no network at all. Not outbound, not
// inbound, not loopback, and unlike Landlock this is not limited to TCP.
func Support() domainplugin.SandboxSupport {
	if !ProfileAPIAvailable() {
		return domainplugin.SandboxSupport{
			Reason: "this Windows build does not provide the AppContainer profile API, so the " +
				"plugin runs with your account's full access to files and the network",
		}
	}
	return domainplugin.SandboxSupport{Available: true, Network: true}
}

// ContainerTempDir is where an AppContainer process will look for its temp directory, given the
// LOCALAPPDATA it was started with.
//
// Windows computes this path itself and overwrites whatever TEMP the parent set, so the only way to
// keep a plugin's temporary files inside its own instance directory — which is the property #76
// established and an OS sandbox must not quietly undo — is to point LOCALAPPDATA at that directory
// and let the container derive a path beneath it. The directory is not created by anyone else: the
// profile only creates it under the real LOCALAPPDATA, so with a substituted one the host has to
// create it before the spawn or the plugin's first temporary file fails with "path not found".
func ContainerTempDir(localAppData, containerName string) string {
	return filepath.Join(localAppData, "Packages", containerName, "AC", "Temp")
}

// ContainerEnv adds what an AppContainer requires to an environment the caller has already built.
//
// LOCALAPPDATA is not optional and not cosmetic: without it CreateProcess fails outright with
// ERROR_ENVVAR_NOT_FOUND, because the container computes its redirected paths from it. The value is
// the plugin's own instance directory rather than the user's real profile path — the container
// never validates it, so nothing forces us to disclose where the user's account lives, and the
// redirected temp directory lands inside the one root the plugin is already granted.
func ContainerEnv(env []string, instanceDataDir string) []string {
	return append(env, "LOCALAPPDATA="+instanceDataDir)
}
