package usecase

import (
	"context"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// BindSession registers a session for plugin session.* RPC authorization.
func (m *PluginManager) BindSession(pluginID, sessionID string) error {
	if m == nil || m.host == nil {
		return domainplugin.ErrSessionNotBound
	}
	return m.host.BindSession(pluginID, sessionID)
}

// UnbindSession removes a session from plugin session.* RPC authorization.
func (m *PluginManager) UnbindSession(pluginID, sessionID string) {
	if m == nil || m.host == nil {
		return
	}
	m.host.UnbindSession(pluginID, sessionID)
}

func (m *PluginManager) sessionScope(pluginID, sessionID string) string {
	scope, err := m.hostProcessScope(pluginID, sessionID)
	if err != nil {
		return ""
	}
	return scope
}

// EnsureRunningForSession starts the plugin process for a session when needed.
func (m *PluginManager) EnsureRunningForSession(ctx context.Context, pluginID, sessionID string) error {
	if !m.isPluginEnabled(pluginID) {
		return domainplugin.ErrPluginDisabled
	}
	scope, err := m.hostProcessScope(pluginID, sessionID)
	if err != nil {
		return err
	}
	if m.host.State(pluginID, scope) == domainplugin.ProcessRunning {
		m.TouchActivity(pluginID)
		return nil
	}
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return err
	}
	m.emitStateChange(pluginID, "starting", sessionID)
	if err := m.host.Start(ctx, plugin, scope); err != nil {
		return err
	}
	m.TouchActivity(pluginID)
	m.emitStateChange(pluginID, "running", sessionID)
	return nil
}

// CallForSession sends RPC to the plugin instance bound to a session.
func (m *PluginManager) CallForSession(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	return m.CallProcess(ctx, pluginID, sessionID, method, params)
}

// NotifyForSession sends a notification to the plugin instance bound to a session.
func (m *PluginManager) NotifyForSession(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
	return m.NotifyProcess(ctx, pluginID, sessionID, method, params)
}

// OnProcessCrashed marks plugin-owned sessions as errored.
func (m *PluginManager) OnProcessCrashed(pluginID, sessionID string) {
	m.emitStateChange(pluginID, "crashed", sessionID)
	if m == nil || m.crashHandler == nil {
		return
	}
	m.crashHandler.HandlePluginProcessCrashed(pluginID, sessionID)
}
