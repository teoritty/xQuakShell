//go:build linux

package plugin

import "sync"

var trackedPluginPIDs sync.Map

// trackPluginPID records a plugin child PID for shutdown reaping.
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

// KillAllTrackedPlugins sends SIGKILL to every tracked plugin process group.
func KillAllTrackedPlugins() {
	trackedPluginPIDs.Range(func(key, _ any) bool {
		pid, ok := key.(int)
		if ok {
			killPluginProcess(pid)
		}
		trackedPluginPIDs.Delete(key)
		return true
	})
}
