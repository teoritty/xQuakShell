package plugin

import domainplugin "xquakshell/internal/domain/plugin"

func (h *ProcessHost) resolveKey(pluginID, sessionID string) string {
	if sessionID != "" {
		return pluginID + "\x00" + sessionID
	}
	return pluginID
}

func (h *ProcessHost) runningProcess(pluginID, sessionID string) (*managedProcess, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	mp, ok := h.processes[h.resolveKey(pluginID, sessionID)]
	if !ok || mp.state != domainplugin.ProcessRunning {
		return nil, domainplugin.ErrPluginNotRunning
	}
	return mp, nil
}

// State returns the current process state for a plugin instance.
func (h *ProcessHost) State(pluginID, sessionID string) domainplugin.ProcessState {
	h.mu.Lock()
	defer h.mu.Unlock()
	mp, ok := h.processes[h.resolveKey(pluginID, sessionID)]
	if !ok {
		return domainplugin.ProcessDiscovered
	}
	return mp.state
}

// RunningInstances returns a snapshot of tracked plugin processes and their states.
func (h *ProcessHost) RunningInstances() []domainplugin.ProcessInstance {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]domainplugin.ProcessInstance, 0, len(h.processes))
	for _, mp := range h.processes {
		out = append(out, domainplugin.ProcessInstance{
			PluginID:  mp.plugin.Manifest.ID,
			SessionID: mp.sessionID,
			State:     mp.state,
		})
	}
	return out
}
