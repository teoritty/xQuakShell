package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// OnEmbedReadyFunc is called when an embed descriptor is registered for a session.
type OnEmbedReadyFunc func(desc domain.SessionEmbedDescriptor)

// SetEmbedTunnelService wires the embed tunnel registry into the session manager.
func (m *SessionManager) SetEmbedTunnelService(svc *EmbedTunnelService) {
	m.embedTunnels = svc
}

// SetOnEmbedReady sets the callback invoked when embed UI becomes available.
func (m *SessionManager) SetOnEmbedReady(fn OnEmbedReadyFunc) {
	m.onEmbedReady = fn
}

// PluginIDForSession returns the plugin that owns an open session.
func (m *SessionManager) PluginIDForSession(sessionID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[sessionID]
	if !ok || entry.pluginID == "" {
		return "", false
	}
	if entry.info.State == domain.SessionClosed {
		return "", false
	}
	return entry.pluginID, true
}

// HandlePluginRegisterEmbed mints an embed token and stores the session descriptor.
func (m *SessionManager) HandlePluginRegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error) {
	if m.embedTunnels == nil {
		return nil, fmt.Errorf("embed tunnel service unavailable")
	}
	m.mu.RLock()
	entry, ok := m.sessions[sessionID]
	if !ok || entry.pluginID != pluginID {
		m.mu.RUnlock()
		return nil, domain.ErrSessionNotFound
	}
	protocol := entry.info.Protocol
	m.mu.RUnlock()

	plugin, err := m.pluginBridge.plugins.Registry().Get(pluginID)
	if err != nil {
		return nil, err
	}
	expected := plugin.Manifest.EmbedEntryForProtocol(protocol)
	uiEntry = strings.TrimSpace(uiEntry)
	if uiEntry == "" {
		uiEntry = expected
	}
	if uiEntry != expected {
		return nil, fmt.Errorf("%w: uiEntry mismatch", domainplugin.ErrInvalidManifest)
	}

	desc, err := m.embedTunnels.Register(ctx, domain.EmbedRegistration{
		SessionID: sessionID,
		PluginID:  pluginID,
		UIEntry:   uiEntry,
		TunnelIDs: tunnelIDs,
		ExpiresAt: time.Now().Add(domain.DefaultEmbedTokenTTL),
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if entry, ok := m.sessions[sessionID]; ok {
		entry.embedDescriptor = &desc
		entry.sessionSurface = "embed"
		entry.info.Surface = "embed"
	}
	m.mu.Unlock()

	resp := map[string]any{
		"embedToken": extractEmbedToken(desc.UIUrl),
		"uiUrl":      desc.UIUrl,
		"tunnelUrl":  desc.TunnelUrl,
		"expiresAt":  desc.ExpiresAt.Format(time.RFC3339),
	}
	return json.Marshal(resp)
}

// HandlePluginTunnelOpen marks a tunnel ready for frame routing.
func (m *SessionManager) HandlePluginTunnelOpen(_ context.Context, pluginID, sessionID, tunnelID string) error {
	if err := m.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	if m.embedTunnels == nil {
		return fmt.Errorf("embed tunnel service unavailable")
	}
	return m.embedTunnels.OpenTunnel(sessionID, tunnelID)
}

// HandlePluginTunnelFrame forwards opaque bytes to the browser WebSocket.
func (m *SessionManager) HandlePluginTunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error {
	if err := m.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	if m.embedTunnels == nil {
		return fmt.Errorf("embed tunnel service unavailable")
	}
	if eof {
		return m.embedTunnels.CloseTunnel(sessionID, tunnelID)
	}
	data, err := decodeTunnelData(dataBase64)
	if err != nil {
		return err
	}
	return m.embedTunnels.RouteTunnelFrameFromPlugin(ctx, sessionID, tunnelID, data)
}

// HandlePluginTunnelClose closes a tunnel for the session.
func (m *SessionManager) HandlePluginTunnelClose(_ context.Context, pluginID, sessionID, tunnelID string) error {
	if err := m.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	if m.embedTunnels == nil {
		return fmt.Errorf("embed tunnel service unavailable")
	}
	return m.embedTunnels.CloseTunnel(sessionID, tunnelID)
}

// HandlePluginReportLocalEmbed rejects Mode B in v1 broker-default builds.
func (m *SessionManager) HandlePluginReportLocalEmbed(_ context.Context, pluginID, sessionID string, _ json.RawMessage) error {
	if err := m.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	return ErrLocalEmbedNotSupported
}

func (m *SessionManager) assertPluginSession(pluginID, sessionID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[sessionID]
	if !ok || entry.pluginID != pluginID {
		return domain.ErrSessionNotFound
	}
	return nil
}

func extractEmbedToken(uiURL string) string {
	const prefix = "/embed/s/"
	idx := strings.Index(uiURL, prefix)
	if idx < 0 {
		return ""
	}
	rest := uiURL[idx+len(prefix):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

var _ PluginEmbedSink = (*SessionManager)(nil)
