//go:build !windows

package plugin

import domainplugin "xquakshell/internal/domain/plugin"

// SweepOrphanContainers has nothing to do off Windows. An AppContainer profile is durable state
// that outlives the process it was made for; a Landlock ruleset is built per spawn and dies with
// the process, so there is nothing to reap and no equivalent to write. That asymmetry is a property
// of the two mechanisms, not an omission here.
func SweepOrphanContainers() {}

// releaseInstanceContainer has nothing to release: a Landlock ruleset dies with the process.
func releaseInstanceContainer(_ domainplugin.InstalledPlugin, _ string) {}
