package usecase

import (
	"context"

	domainplugin "xquakshell/internal/domain/plugin"
)

// SetStartGuard registers a guard taken before a plugin process starts and released, through the
// function it returns, once the start has finished. Plugin data sync uses it to apply incoming files
// while nothing can read them, and to keep a concurrent pass from writing under a starting process.
func (m *PluginManager) SetStartGuard(guard func(ctx context.Context, pluginID string) func()) {
	m.startGuard = guard
}

// SetProcessIdleHandler registers an observer of a plugin's process going away on purpose - stopped
// or suspended - the moments its data directory becomes safe to write. It runs on the caller's
// goroutine and must hand slow work off.
func (m *PluginManager) SetProcessIdleHandler(fn func(pluginID string)) {
	m.processIdle = fn
}

// SetInstalledChangedHandler registers an observer of a plugin installed or uninstalled here, so the
// plugins the user's other devices are offered follow at once rather than on the next scan. It runs
// on the installing goroutine and must hand slow work off.
func (m *PluginManager) SetInstalledChangedHandler(fn func()) {
	m.installedChanged = fn
}

func (m *PluginManager) notifyInstalledChanged() {
	if m.installedChanged != nil {
		m.installedChanged()
	}
}

// ProcessIdle reports that no process of the plugin is running, starting, stopping, or crashed and
// awaiting the supervisor's restart, which does not pass through the start guard.
func (m *PluginManager) ProcessIdle(pluginID string) bool {
	switch m.aggregateProcessState(pluginID) {
	case domainplugin.ProcessDiscovered, domainplugin.ProcessStopped, domainplugin.ProcessSuspended:
		return true
	}
	return false
}

// SyncedManifests lists the installed plugins' manifests.
func (m *PluginManager) SyncedManifests() []domainplugin.Manifest {
	installed := m.registry.List()
	out := make([]domainplugin.Manifest, 0, len(installed))
	for _, p := range installed {
		out = append(out, p.Manifest)
	}
	return out
}

// RestartPlugin stops a plugin and starts it again, for the sync panel's "updates waiting" Restart:
// the stop makes the data directory writable and the start passes through the start guard, which
// applies what arrived before the new process can read it. The start is EnsureRunning, as after an
// install, rather than StartPluginManual: the plugin was running already, and a manual start would
// refuse every plugin that does not declare the manual activation event.
func (m *PluginManager) RestartPlugin(ctx context.Context, pluginID string) error {
	if _, err := m.registry.Get(pluginID); err != nil {
		return err
	}
	if err := m.StopPlugin(ctx, pluginID); err != nil {
		return err
	}
	return m.EnsureRunning(ctx, pluginID)
}
