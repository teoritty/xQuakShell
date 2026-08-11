//go:build linux

package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
	"xquakshell/internal/pkg/pathsafe"
)

// resolveSpawnTarget decides whether this plugin process goes through the Landlock shim, and builds
// the instruction it will be given.
//
// A kernel that cannot confine anything gets the plain exec it always got: the platform's inability
// is not a failure and does not stop a plugin from starting. A kernel that can and then fails to is
// a different case entirely — it is an error here, and the caller decides what to do about it,
// because silently exec'ing unconfined after asking for confinement is how a sandbox stops working
// for a fraction of users with nobody noticing.
func resolveSpawnTarget(dataRoot string, plugin domainplugin.InstalledPlugin, entryPath, instanceDataDir string) (spawnTarget, error) {
	if !sandbox.Support().Available {
		return spawnTarget{path: entryPath}, nil
	}
	self, err := os.Executable()
	if err != nil {
		return spawnTarget{}, fmt.Errorf("locate host binary for the plugin sandbox: %w", err)
	}
	readable, err := installReadPaths(dataRoot, plugin)
	if err != nil {
		return spawnTarget{}, err
	}
	args := sandbox.EncodeShimArgs(sandbox.ShimArgs{
		DataRoot: PluginsRoot(dataRoot),
		AllowRX:  readable,
		AllowRW:  []string{instanceDataDir},
		Exec:     entryPath,
	})
	// The shim parses this again and refuses what it cannot vouch for. Parsing it here as well
	// turns a mistake into a start error naming the plugin, instead of a child process that dies
	// before it can say anything the host will attribute to the right plugin.
	if _, err := sandbox.ParseShimArgs(append([]string{self}, args...)); err != nil {
		return spawnTarget{}, fmt.Errorf("plugin %s sandbox: %w", plugin.Manifest.ID, err)
	}
	return spawnTarget{path: self, args: args}, nil
}

// installReadPaths is the plugin's install tree, granted entry by entry rather than as one
// directory.
//
// A user-installed plugin's data directory sits INSIDE its install tree
// (<dataRoot>/plugins/<id>/data next to <dataRoot>/plugins/<id>/bin), and a single grant on the
// install directory would carry read access to all of it — so a per-session plugin would be able to
// read every other session's instance directory, reopening precisely the isolation ADR-003
// established. Enumerating the entries and dropping the data one closes that, and costs a readdir
// per start on a directory with a handful of entries in it.
//
// The price is that the plugin cannot list its own install directory, only read within its
// subtrees. Nothing in the plugin contract asks it to, and a plugin that does gets a denial it can
// see rather than a sibling's files it should not.
func installReadPaths(dataRoot string, plugin domainplugin.InstalledPlugin) ([]string, error) {
	// The entry path was resolved through its symlinks, so the install root has to be too or the
	// containment check between them compares two different spellings of the same directory.
	root, err := filepath.EvalSymlinks(plugin.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin %s install dir: %w", plugin.Manifest.ID, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read plugin %s install dir: %w", plugin.Manifest.ID, err)
	}
	dataBase := PluginDataDir(dataRoot, plugin.Manifest.ID)

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if pathsafe.UnderRoot(path, dataBase) {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("plugin %s install dir has nothing to grant", plugin.Manifest.ID)
	}
	return paths, nil
}
