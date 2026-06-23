package plugin

import (
	domainplugin "ssh-client/internal/domain/plugin"
)

// BindSession registers an active session authorized for session.* RPC from this plugin.
func (h *ProcessHost) BindSession(pluginID, sessionID string) error {
	if h == nil || h.cfg.SessionAuthorizer == nil {
		return domainplugin.ErrSessionNotBound
	}
	return h.cfg.SessionAuthorizer.BindSession(pluginID, sessionID)
}

// UnbindSession removes a session from the plugin authorization registry.
func (h *ProcessHost) UnbindSession(pluginID, sessionID string) {
	if h == nil || h.cfg.SessionAuthorizer == nil {
		return
	}
	h.cfg.SessionAuthorizer.UnbindSession(pluginID, sessionID)
}
