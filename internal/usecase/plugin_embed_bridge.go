package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginEmbedBridge forwards host-initiated embed notifications to plugins.
type PluginEmbedBridge struct {
	plugins *PluginManager
	tunnels *EmbedTunnelService
	lookup  EmbedSessionLookup
}

// EmbedSessionLookup resolves plugin ownership for open sessions.
type EmbedSessionLookup interface {
	PluginIDForSession(sessionID string) (string, bool)
}

// NewPluginEmbedBridge creates a bridge for embed viewport and activity notifications.
func NewPluginEmbedBridge(plugins *PluginManager, tunnels *EmbedTunnelService, lookup EmbedSessionLookup) *PluginEmbedBridge {
	return &PluginEmbedBridge{plugins: plugins, tunnels: tunnels, lookup: lookup}
}

// ReportViewport forwards pixel dimensions to the plugin process.
func (b *PluginEmbedBridge) ReportViewport(ctx context.Context, sessionID string, widthPx, heightPx int, dpr float64, active bool) error {
	if b == nil || b.plugins == nil {
		return nil
	}
	pluginID, ok := b.pluginIDForSession(sessionID)
	if !ok {
		return domain.ErrSessionNotFound
	}
	params, _ := json.Marshal(map[string]any{
		"sessionId":        sessionID,
		"widthPx":          widthPx,
		"heightPx":         heightPx,
		"devicePixelRatio": dpr,
		"active":           active,
	})
	return b.plugins.NotifyForSession(ctx, pluginID, sessionID, "session.embedViewport", params)
}

// ReportActivity updates broker backpressure and notifies the plugin.
func (b *PluginEmbedBridge) ReportActivity(ctx context.Context, sessionID string, active bool) error {
	if b == nil {
		return nil
	}
	if b.tunnels != nil {
		b.tunnels.SetSessionActive(sessionID, active)
	}
	if b.plugins == nil {
		return nil
	}
	pluginID, ok := b.pluginIDForSession(sessionID)
	if !ok {
		return domain.ErrSessionNotFound
	}
	params, _ := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"active":    active,
	})
	if err := b.plugins.NotifyForSession(ctx, pluginID, sessionID, "session.embedActivity", params); err != nil {
		return err
	}
	if !active {
		return b.plugins.NotifyForSession(ctx, pluginID, sessionID, "session.tunnelBackpressure", json.RawMessage(`{"sessionId":"`+sessionID+`"}`))
	}
	return b.plugins.NotifyForSession(ctx, pluginID, sessionID, "session.tunnelResume", json.RawMessage(`{"sessionId":"`+sessionID+`"}`))
}

func (b *PluginEmbedBridge) pluginIDForSession(sessionID string) (string, bool) {
	if b.lookup == nil {
		return "", false
	}
	return b.lookup.PluginIDForSession(sessionID)
}

// ErrLocalEmbedNotSupported indicates Mode B is unavailable in this build path.
var ErrLocalEmbedNotSupported = fmt.Errorf("%w: local embed server not supported", domainplugin.ErrNotImplemented)
