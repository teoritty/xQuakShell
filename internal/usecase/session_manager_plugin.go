package usecase

import "context"
// PluginBridge returns the plugin session bridge for wiring plugin IPC handlers.
func (m *SessionManager) PluginBridge() *PluginSessionBridge {
	return m.pluginBridge
}

// HandlePluginUpdateState applies plugin-reported session state (IDOR-checked).
func (m *SessionManager) HandlePluginUpdateState(pluginID, sessionID, state, errMsg string) error {
	return m.pluginBridge.HandlePluginUpdateState(pluginID, sessionID, state, errMsg)
}

// HandlePluginWriteTerminal pushes terminal bytes from a plugin into the UI stream.
func (m *SessionManager) HandlePluginWriteTerminal(pluginID, sessionID string, data []byte) error {
	return m.pluginBridge.HandlePluginWriteTerminal(pluginID, sessionID, data)
}

// PluginOwnsConnection reports whether the plugin has an active session for the connection.
func (m *SessionManager) PluginOwnsConnection(pluginID, connectionID string) bool {
	return m.pluginBridge.PluginOwnsConnection(pluginID, connectionID)
}

// PluginOwnsSession reports whether the plugin owns an active session by ID.
func (m *SessionManager) PluginOwnsSession(pluginID, sessionID string) bool {
	return m.pluginBridge.PluginOwnsSession(pluginID, sessionID)
}

// BindPluginSessionForTest assigns plugin ownership on a session (unit tests).
func (m *SessionManager) BindPluginSessionForTest(sessionID, pluginID string, outputBuffer ...int) error {
	return m.pluginBridge.BindPluginSessionForTest(sessionID, pluginID, outputBuffer...)
}

// HandlePluginProcessCrashed marks plugin-owned sessions for recovery.
func (m *SessionManager) HandlePluginProcessCrashed(pluginID, sessionID string) {
	m.pluginBridge.HandlePluginProcessCrashed(pluginID, sessionID)
}

// RecoverPluginSession re-sends session.connect after a plugin process restart.
func (m *SessionManager) RecoverPluginSession(ctx context.Context, pluginID, sessionID string) error {
	return m.pluginBridge.RecoverPluginSession(ctx, pluginID, sessionID)
}

// FailPluginSessionRecovery marks a session as failed after recovery attempts are exhausted.
func (m *SessionManager) FailPluginSessionRecovery(pluginID, sessionID string) {
	m.pluginBridge.FailPluginSessionRecovery(pluginID, sessionID)
}
