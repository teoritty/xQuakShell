package plugin

// ReleaseTunnelDialSlot releases a tunnel.dial concurrency slot for a plugin session process.
func (h *ProcessHost) ReleaseTunnelDialSlot(pluginID, sessionID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	mp := h.processes[h.resolveKey(pluginID, sessionID)]
	h.mu.Unlock()
	if mp != nil && mp.releaseDialSlot != nil {
		mp.releaseDialSlot()
	}
}
