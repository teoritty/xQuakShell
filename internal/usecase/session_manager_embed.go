package usecase

import (
	"context"
	"encoding/json"

	domainplugin "xquakshell/internal/domain/plugin"
)

// OnEmbedReadyFunc is called when an embed descriptor is registered for a session.
type OnEmbedReadyFunc = EmbedReadyFunc

// SetForwardRuleValidator wires forward rule validation at connect time.
func (m *SessionManager) SetForwardRuleValidator(v *ForwardRuleValidator) {
	if m != nil && m.lifecycle != nil {
		m.lifecycle.SetForwardRuleValidator(v)
	}
}

// SetDynamicForward wires dynamic port-forward coordination into session lifecycle.
func (m *SessionManager) SetDynamicForward(c *DynamicForwardCoordinator) {
	if m != nil && m.lifecycle != nil {
		m.lifecycle.dynamicForward = c
	}
}

// SetEmbedTunnelService wires the embed tunnel registry into the session manager.
func (m *SessionManager) SetEmbedTunnelService(svc *EmbedTunnelService) {
	m.embed = svc
	m.lifecycle.SetEmbed(svc)
	var lookup PluginManifestLookup
	if m.plugins != nil && m.plugins.plugins != nil {
		lookup = m.plugins.plugins.Registry()
	}
	svc.WireSessionContext(m.registry, lookup)
}

// WireChannelSessionRegistry hands the session registry to the composition root's channel
// resolver, which is built before this SessionManager exists and therefore cannot be given it as a
// constructor argument. The registry is pushed to the sink rather than returned by an accessor:
// the exec channel backend is the only channel-side collaborator that needs it, and SessionRegistry
// stays unexported to everything else (ADR-009 lock discipline -- Get is the only sanctioned door).
func (m *SessionManager) WireChannelSessionRegistry(sink func(*SessionRegistry)) {
	if m == nil || sink == nil {
		return
	}
	sink(m.registry)
}

// SetChannelBus wires the channel bus into session lifecycle for close cascade.
func (m *SessionManager) SetChannelBus(bus domainplugin.ChannelSessionCloser) {
	if m != nil && m.lifecycle != nil {
		m.lifecycle.SetChannelBus(bus)
	}
}

// SetDiscovery wires the discovery leader tracker into session lifecycle (ADR-014).
func (m *SessionManager) SetDiscovery(tracker DiscoverySessionTracker) {
	if m != nil && m.lifecycle != nil {
		m.lifecycle.SetDiscovery(tracker)
	}
}

// SetOnEmbedReady sets the callback invoked when embed UI becomes available.
func (m *SessionManager) SetOnEmbedReady(fn OnEmbedReadyFunc) {
	if m.embed != nil {
		m.embed.SetEmbedReadyHandler(fn)
	}
}

// PluginIDForSession returns the plugin that owns an open session.
func (m *SessionManager) PluginIDForSession(sessionID string) (string, bool) {
	if m.embed != nil {
		return m.embed.PluginIDForSession(sessionID)
	}
	return "", false
}

// HandlePluginRegisterEmbed mints an embed token and stores the session descriptor.
func (m *SessionManager) HandlePluginRegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error) {
	return m.embed.HandlePluginRegisterEmbed(ctx, pluginID, sessionID, uiEntry, tunnelIDs)
}

// HandlePluginTunnelOpen marks a tunnel ready for frame routing.
func (m *SessionManager) HandlePluginTunnelOpen(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	return m.embed.HandlePluginTunnelOpen(ctx, pluginID, sessionID, tunnelID)
}

func (m *SessionManager) HandlePluginTunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error {
	return m.embed.HandlePluginTunnelFrame(ctx, pluginID, sessionID, tunnelID, dataBase64, eof)
}

func (m *SessionManager) HandlePluginTunnelClose(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	return m.embed.HandlePluginTunnelClose(ctx, pluginID, sessionID, tunnelID)
}

func (m *SessionManager) HandlePluginReportLocalEmbed(ctx context.Context, pluginID, sessionID string, raw json.RawMessage) error {
	return m.embed.HandlePluginReportLocalEmbed(ctx, pluginID, sessionID, raw)
}
