package usecase

import "context"

func (m *SessionManager) HandlePluginUpdateState(pluginID, sessionID, state, errMsg string) error {
	return m.plugins.HandlePluginUpdateState(pluginID, sessionID, state, errMsg)
}

func (m *SessionManager) HandlePluginWriteTerminal(pluginID, sessionID string, data []byte) error {
	return m.plugins.HandlePluginWriteTerminal(pluginID, sessionID, data)
}

func (m *SessionManager) PluginOwnsConnection(pluginID, connectionID string) bool {
	return m.plugins.PluginOwnsConnection(pluginID, connectionID)
}

func (m *SessionManager) PluginOwnsSession(pluginID, sessionID string) bool {
	return m.plugins.PluginOwnsSession(pluginID, sessionID)
}

// TODO(test-hooks): ships in the release binary because external test/unit/plugin packages
// (IDOR tests) call it; removing it needs a rework of that test harness.
func (m *SessionManager) BindPluginSessionForTest(sessionID, pluginID string, outputBuffer ...int) error {
	return m.plugins.BindPluginSessionForTest(sessionID, pluginID, outputBuffer...)
}

func (m *SessionManager) HandlePluginProcessCrashed(pluginID, sessionID string) {
	m.plugins.HandlePluginProcessCrashed(pluginID, sessionID)
}

func (m *SessionManager) RecoverPluginSession(ctx context.Context, pluginID, sessionID string) error {
	return m.plugins.RecoverPluginSession(ctx, pluginID, sessionID)
}

func (m *SessionManager) FailPluginSessionRecovery(pluginID, sessionID string) {
	m.plugins.FailPluginSessionRecovery(pluginID, sessionID)
}
