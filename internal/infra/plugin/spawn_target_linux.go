//go:build linux

package plugin

import (
	"fmt"
	"os"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
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
