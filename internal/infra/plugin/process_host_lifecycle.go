package plugin

import (
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
)

func (h *ProcessHost) waitProcess(key string, mp *managedProcess) {
	<-mp.reaper.Done()
	exitErr := mp.reaper.ExitErr()
	mp.closeResources(false)

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

// finalizeProcess releases IPC and job resources without calling Wait (reaper owns Wait).
func (h *ProcessHost) finalizeProcess(key string, mp *managedProcess) {
	mp.closeResources(true)

	h.mu.Lock()
	if current, ok := h.processes[key]; ok && current == mp {
		delete(h.processes, key)
	}
	h.mu.Unlock()
}
