package plugin

import (
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

func (h *ProcessHost) waitProcess(key string, mp *managedProcess) {
	<-mp.reaper.Done()
	exitErr := mp.reaper.ExitErr()
	mp.closeResources(false)
	h.unregisterChannels(key, mp)

	h.mu.Lock()
	defer h.mu.Unlock()

	current, ok := h.processes[key]
	if !ok || current != mp {
		return
	}

	crashed := exitErr != nil
	if crashed {
		mp.state = domainplugin.ProcessCrashed
		slog.Warn("plugin process exited", "pluginId", mp.plugin.Manifest.ID, "sessionId", mp.sessionID, "err", exitErr)
	} else if mp.state != domainplugin.ProcessStopping {
		mp.state = domainplugin.ProcessStopped
	}
	delete(h.processes, key)
	if mp.cmd != nil && mp.cmd.Process != nil {
		untrackPluginPID(mp.cmd.Process.Pid)
	}

	if crashed && h.cfg.OnCrash != nil {
		h.cfg.OnCrash(mp.plugin.Manifest.ID, mp.sessionID)
	}
}

// releaseStartReservation drops a ProcessStarting reservation after a failed Start.
func (h *ProcessHost) releaseStartReservation(key string, mp *managedProcess) {
	h.finalizeProcess(key, mp)
}

// unregisterChannels takes THIS process's ChannelProxy off the bus, and only this one.
//
// Both teardown paths used to unregister by key alone, before checking whether the key still
// belonged to them — so a crashed process finishing its teardown after a restart had already
// claimed the key would deregister the new, running process, leaving its channels unreachable from
// the session-close cascade. The identity is carried into the bus rather than checked here on
// purpose: checking here would leave the check and the delete non-atomic unless h.mu were held
// across the bus call, which would nest the two mutexes.
func (h *ProcessHost) unregisterChannels(key string, mp *managedProcess) {
	if h.cfg.ChannelBus == nil {
		return
	}
	h.mu.Lock()
	proxy := mp.channels
	h.mu.Unlock()
	h.cfg.ChannelBus.Unregister(key, proxy)
}

// finalizeProcess releases IPC and job resources without calling Wait (reaper owns Wait).
func (h *ProcessHost) finalizeProcess(key string, mp *managedProcess) {
	mp.closeResources(true)
	h.unregisterChannels(key, mp)

	h.mu.Lock()
	if current, ok := h.processes[key]; ok && current == mp {
		delete(h.processes, key)
	}
	h.mu.Unlock()
}
