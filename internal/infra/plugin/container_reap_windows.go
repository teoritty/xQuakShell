//go:build windows

package plugin

import (
	"log/slog"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// sweepOnce keeps the sweep to one pass per process.
//
// It is startup housekeeping, and NewProcessHost is not startup: a test suite builds dozens of
// hosts, and each one re-enumerating and re-deleting the same registry keys is both wasted work and
// a way for two hosts in the same run to trip over each other's profiles.
var sweepOnce sync.Once

// SweepOrphanContainers deletes every AppContainer profile this application created and did not
// clean up.
//
// A profile is a registry write and a directory that persist until something removes them, and a
// per-session container means one profile per session — so an installation that only ever created
// them would accumulate thousands over its life. The normal teardown removes each one as its
// process ends; this covers what a crash or a power loss skipped, and it runs at startup because
// that is the one moment when none of ours can be in use.
//
// "None of ours can be in use" is true for this process and assumes the user is not running a
// second copy of the application. If they are, its plugins keep working: deleting a profile does
// not revoke a running process's token, its ACEs name a SID derived from the name rather than from
// the profile, and the temp directory the plugin actually uses lives under its own instance
// directory. What that copy loses is the Packages folder Windows created for it, which nothing here
// reads. The one interaction that could have failed a start — a delete landing between another
// instance's create and its spawn — is handled where it belongs, in CreateContainer.
//
// Failures are logged and not returned. This is housekeeping; a profile that will not delete is not
// a reason to refuse to start.
func SweepOrphanContainers() {
	sweepOnce.Do(sweepOrphanContainers)
}

func sweepOrphanContainers() {
	if !sandbox.Support().Available {
		return
	}
	names, err := sandbox.ListContainerNames(sandbox.ContainerNamePrefix)
	if err != nil {
		slog.Warn("plugin sandbox: could not enumerate app container profiles", "err", err)
		return
	}
	for _, name := range names {
		if err := sandbox.DeleteContainer(name); err != nil {
			slog.Warn("plugin sandbox: could not delete an orphaned app container profile",
				"profile", name, "err", err)
			continue
		}
		slog.Debug("plugin sandbox: deleted an orphaned app container profile", "profile", name)
	}
}

// releaseInstanceContainer deletes one instance's profile as its process goes away.
//
// This is the path that keeps profiles and their ACEs from accumulating in normal operation; the
// startup sweep is only for what a crash skipped. It is safe for a plugin that will start again —
// the name is a pure function of the identity, so a per-plugin instance recreates the same profile
// with the same SID and the ACEs already written for it stay valid in the meantime.
//
// dataRoot is here so the ACEs can be found: they were written on paths derived from it, and a
// teardown that knows only the identity can delete the profile while leaving the marks it made.
func releaseInstanceContainer(plugin domainplugin.InstalledPlugin, sessionID, dataRoot string) {
	if !sandbox.Support().Available {
		return
	}
	if err := releaseContainer(plugin, dataRoot, sessionID); err != nil {
		slog.Warn("plugin sandbox: could not delete an app container profile",
			"pluginId", plugin.Manifest.ID, "err", err)
	}
}
