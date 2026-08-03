package usecase

// RegisterViewPanel marks a plugin WebView panel as mounted (prevents idle suspend).
func (m *PluginManager) RegisterViewPanel(pluginID, panelID string) {
	if m == nil || pluginID == "" || panelID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.viewPanelCounts == nil {
		m.viewPanelCounts = make(map[string]int)
	}
	m.viewPanelCounts[pluginID]++
	m.touchActivityLocked(pluginID)
}

// UnregisterViewPanel marks a plugin WebView panel as unmounted.
func (m *PluginManager) UnregisterViewPanel(pluginID, panelID string) {
	if m == nil || pluginID == "" || panelID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.viewPanelCounts == nil {
		return
	}
	if count := m.viewPanelCounts[pluginID]; count <= 1 {
		delete(m.viewPanelCounts, pluginID)
	} else {
		m.viewPanelCounts[pluginID] = count - 1
	}
}

func (m *PluginManager) hasActiveViewPanels(pluginID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hasActiveViewPanelsLocked(pluginID)
}

func (m *PluginManager) hasActiveViewPanelsLocked(pluginID string) bool {
	if m == nil || m.viewPanelCounts == nil {
		return false
	}
	return m.viewPanelCounts[pluginID] > 0
}
