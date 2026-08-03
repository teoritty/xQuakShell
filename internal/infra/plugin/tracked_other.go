//go:build !linux

package plugin

import "sync"

var trackedPluginPIDs sync.Map

// trackPluginPID records a plugin child PID for shutdown reaping (no-op off Linux).
func trackPluginPID(pid int) {
	if pid > 0 {
		trackedPluginPIDs.Store(pid, struct{}{})
	}
}

// untrackPluginPID removes a plugin child PID after it has exited.
func untrackPluginPID(pid int) {
	if pid > 0 {
		trackedPluginPIDs.Delete(pid)
	}
}

// KillAllTrackedPlugins is a no-op off Linux (process reaper handles shutdown).
func KillAllTrackedPlugins() {}
