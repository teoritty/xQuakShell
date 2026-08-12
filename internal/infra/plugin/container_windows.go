//go:build windows

package plugin

import (
	"fmt"
	"log/slog"
	"os"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// pluginContainerName is the AppContainer profile name for one plugin process instance.
//
// It goes through PluginInstanceKey so that the container and the instance data directory are
// scoped to exactly the same thing. Under per-session isolation that is (plugin, session); under
// per-plugin it is the plugin alone. Getting this wrong in the direction of coarser would put two
// sessions' ACEs on one SID and let each read the other's files — the isolation ADR-003 exists to
// provide, removed by the mechanism added to reinforce it.
func pluginContainerName(plugin domainplugin.InstalledPlugin, sessionID string) string {
	key := PluginInstanceKey(plugin.Manifest.ID, sessionID, plugin.Manifest.EffectiveIsolation())
	return sandbox.ContainerName(key)
}

// prepareContainer creates this instance's AppContainer and makes the two directories it needs
// reachable from inside it.
//
// Everything here is idempotent, because a plugin starts many times over an installation's life and
// each start finds most of this already done. The profile is adopted if it exists; the grants are
// verified and only written when missing, since rewriting a directory ACL is O(files) and would be
// paid on every start of every plugin to change nothing.
func prepareContainer(plugin domainplugin.InstalledPlugin, dataRoot, sessionID, instanceDataDir string) (*sandbox.Container, error) {
	name := pluginContainerName(plugin, sessionID)
	container, err := sandbox.CreateContainer(name,
		"xQuakShell plugin "+plugin.Manifest.ID,
		"Isolation for the xQuakShell plugin "+plugin.Manifest.ID)
	if err != nil {
		return nil, fmt.Errorf("create app container for plugin %s: %w", plugin.Manifest.ID, err)
	}

	if err := grantContainerAccess(container, plugin, dataRoot, instanceDataDir); err != nil {
		return nil, err
	}
	// The container will compute this path as its TEMP and Windows creates it only under the real
	// LOCALAPPDATA, which this design deliberately does not hand over. Without it the plugin's
	// first temporary file fails with "path not found".
	if err := os.MkdirAll(sandbox.ContainerTempDir(instanceDataDir, name), 0o700); err != nil {
		return nil, fmt.Errorf("create app container temp dir for plugin %s: %w", plugin.Manifest.ID, err)
	}
	return container, nil
}

// grantContainerAccess gives the container the two things a plugin needs and nothing else.
//
// The install tree is granted entry by entry, exactly as the Landlock ruleset grants it, and for
// exactly the same reason: the data directory is a child of the install directory, and an ACE
// written on the parent is inherited all the way down — so one grant on the install tree would put
// this session's SID on every other session's instance directory.
func grantContainerAccess(container *sandbox.Container, plugin domainplugin.InstalledPlugin, dataRoot, instanceDataDir string) error {
	readable, err := installReadPaths(dataRoot, plugin)
	if err != nil {
		return err
	}
	for _, path := range readable {
		if err := container.EnsureGrant(path, sandbox.AccessReadExecute); err != nil {
			return fmt.Errorf("grant plugin %s read access to %s: %w", plugin.Manifest.ID, path, err)
		}
	}
	if err := container.EnsureGrant(instanceDataDir, sandbox.AccessReadWrite); err != nil {
		return fmt.Errorf("grant plugin %s write access to its data directory: %w", plugin.Manifest.ID, err)
	}
	return nil
}

// releaseContainer deletes the profile for one instance, and takes its ACEs back with it when the
// identity behind them will never be used again.
//
// Profiles persist in the user's registry until deleted, and a per-session container means one
// profile per session, so a long-running installation that never reaped would accumulate thousands
// of them. Every teardown path calls this and they overlap by design; deleting a profile that is
// already gone is success.
//
// The ACEs are only revoked for a session-scoped identity, and the asymmetry is the point. A
// per-plugin container keeps one name for the life of the installation, so its ACE is written once
// and matches on every later start — revoking it would buy nothing and cost a full subtree ACL
// rewrite on every single start, which is exactly what EnsureGrant exists to avoid. A session
// scoped one is used by one connection and then never again, so its ACE is pure residue.
//
// An empty dataRoot means the caller never got as far as owning a running process — a start that
// lost its reservation to a concurrent stop tears down an instance it never adopted. There is
// nothing to revoke from a root nobody named, and guessing one would revoke against whatever
// relative path it resolved to, so this deletes the profile and stops there.
func releaseContainer(plugin domainplugin.InstalledPlugin, dataRoot, sessionID string) error {
	if dataRoot != "" && instanceSessionScope(sessionID, plugin.Manifest.EffectiveIsolation()) != "" {
		revokeContainerAccess(plugin, dataRoot, sessionID)
	}
	return sandbox.DeleteContainer(pluginContainerName(plugin, sessionID))
}

// revokeContainerAccess takes back the two grants prepareContainer wrote.
//
// It addresses the container by name rather than by holding the one the spawn made: the SID is a
// pure function of the name, so the teardown does not have to be handed anything the start created,
// and a start that failed halfway through its grants is cleaned up by the same code.
//
// Nothing here is fatal. A plugin being uninstalled has already lost the directory the ACE was
// written on, and an ACE on a directory that no longer exists is not a leak; a revoke that fails for
// any other reason must still not stop the profile deletion behind it, or a failure that leaves
// residue would also leave the registry entry that residue is measured against.
func revokeContainerAccess(plugin domainplugin.InstalledPlugin, dataRoot, sessionID string) {
	container, err := sandbox.OpenContainer(pluginContainerName(plugin, sessionID))
	if err != nil {
		slog.Warn("plugin sandbox: could not address an app container to revoke its access",
			"pluginId", plugin.Manifest.ID, "err", err)
		return
	}
	granted, err := installReadPaths(dataRoot, plugin)
	if err != nil {
		granted = nil
	}
	isolation := plugin.Manifest.EffectiveIsolation()
	granted = append(granted, PluginInstanceDataDir(dataRoot, plugin.Manifest.ID, sessionID, isolation))
	for _, path := range granted {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := container.Revoke(path); err != nil {
			slog.Warn("plugin sandbox: could not revoke an app container's access",
				"pluginId", plugin.Manifest.ID, "path", path, "err", err)
		}
	}
}
