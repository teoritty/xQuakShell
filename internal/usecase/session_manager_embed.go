package usecase

import (
	"context"
	"encoding/json"

	"ssh-client/internal/domain"
)

// OnEmbedReadyFunc is called when an embed descriptor is registered for a session.
type OnEmbedReadyFunc = EmbedReadyFunc

// SetEmbedTunnelService wires the embed tunnel registry into the session manager.
func (m *SessionManager) SetEmbedTunnelService(svc *EmbedTunnelService) {
	m.embedTunnels = svc
	var lookup PluginManifestLookup
	if m.pluginBridge != nil && m.pluginBridge.plugins != nil {
		lookup = m.pluginBridge.plugins.Registry()
	}
	svc.WireSessionContext(m.registry, lookup)
}

// SetOnEmbedReady sets the callback invoked when embed UI becomes available.
func (m *SessionManager) SetOnEmbedReady(fn OnEmbedReadyFunc) {
	m.onEmbedReady = fn
	if m.embedTunnels != nil {
		m.embedTunnels.SetEmbedReadyHandler(fn)
	}
}

// PluginIDForSession returns the plugin that owns an open session.
func (m *SessionManager) PluginIDForSession(sessionID string) (string, bool) {
	if m.embedTunnels != nil {
		return m.embedTunnels.PluginIDForSession(sessionID)
	}
	entry, ok := m.registry.Get(sessionID)
	if !ok || entry.pluginID == "" || entry.info.State == domain.SessionClosed {
		return "", false
	}
	return entry.pluginID, true
}

// HandlePluginRegisterEmbed mints an embed token and stores the session descriptor.
func (m *SessionManager) HandlePluginRegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error) {
	return m.embedTunnels.HandlePluginRegisterEmbed(ctx, pluginID, sessionID, uiEntry, tunnelIDs)
}

// HandlePluginTunnelOpen marks a tunnel ready for frame routing.
func (m *SessionManager) HandlePluginTunnelOpen(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	return m.embedTunnels.HandlePluginTunnelOpen(ctx, pluginID, sessionID, tunnelID)
}

// HandlePluginTunnelFrame forwards opaque bytes to the browser WebSocket.
func (m *SessionManager) HandlePluginTunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error {
	return m.embedTunnels.HandlePluginTunnelFrame(ctx, pluginID, sessionID, tunnelID, dataBase64, eof)
}

// HandlePluginTunnelClose closes a tunnel for the session.
func (m *SessionManager) HandlePluginTunnelClose(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	return m.embedTunnels.HandlePluginTunnelClose(ctx, pluginID, sessionID, tunnelID)
}

// HandlePluginReportLocalEmbed rejects Mode B in v1 broker-default builds.
func (m *SessionManager) HandlePluginReportLocalEmbed(ctx context.Context, pluginID, sessionID string, raw json.RawMessage) error {
	return m.embedTunnels.HandlePluginReportLocalEmbed(ctx, pluginID, sessionID, raw)
}
