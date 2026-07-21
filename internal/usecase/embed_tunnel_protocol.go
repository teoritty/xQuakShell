package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginIDForSession returns the plugin that owns an open session.
func (s *EmbedTunnelService) PluginIDForSession(sessionID string) (string, bool) {
	if s == nil || s.registry == nil {
		return "", false
	}
	entry, ok := s.registry.Get(sessionID)
	if !ok || entry.pluginID == "" {
		return "", false
	}
	if entry.info.State == domain.SessionClosed {
		return "", false
	}
	return entry.pluginID, true
}

// HandlePluginRegisterEmbed mints an embed token and stores the session descriptor.
func (s *EmbedTunnelService) HandlePluginRegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error) {
	if s == nil {
		return nil, fmt.Errorf("embed tunnel service unavailable")
	}
	var protocol string
	if s.registry == nil || !s.registry.View(sessionID, func(entry *sessionEntry) {
		if entry.pluginID == pluginID {
			protocol = entry.info.Protocol
		}
	}) || protocol == "" {
		return nil, domain.ErrSessionNotFound
	}

	if s.manifestLookup == nil {
		return nil, fmt.Errorf("embed manifest lookup unavailable")
	}
	expected, err := s.manifestLookup.EmbedEntryForProtocol(pluginID, protocol)
	if err != nil {
		return nil, err
	}
	uiEntry = strings.TrimSpace(uiEntry)
	if uiEntry == "" {
		uiEntry = expected
	}
	if uiEntry != expected {
		return nil, fmt.Errorf("%w: uiEntry mismatch", domainplugin.ErrInvalidManifest)
	}

	desc, err := s.Register(ctx, domain.EmbedRegistration{
		SessionID: sessionID,
		PluginID:  pluginID,
		UIEntry:   uiEntry,
		TunnelIDs: tunnelIDs,
		ExpiresAt: time.Now().Add(domain.DefaultEmbedTokenTTL),
	})
	if err != nil {
		return nil, err
	}

	s.registry.Mutate(sessionID, func(entry *sessionEntry) {
		entry.embedDescriptor = &desc
		entry.sessionSurface = "embed"
		entry.info.Surface = "embed"
	})

	resp := map[string]any{
		"embedToken": extractEmbedToken(desc.UIUrl),
		"uiUrl":      desc.UIUrl,
		"tunnelUrl":  desc.TunnelUrl,
		"expiresAt":  desc.ExpiresAt.Format(time.RFC3339),
	}
	return json.Marshal(resp)
}

// HandlePluginTunnelOpen marks a tunnel ready for frame routing.
func (s *EmbedTunnelService) HandlePluginTunnelOpen(_ context.Context, pluginID, sessionID, tunnelID string) error {
	if err := s.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	return s.OpenTunnel(sessionID, tunnelID)
}

// HandlePluginTunnelFrame forwards opaque bytes to the browser WebSocket.
func (s *EmbedTunnelService) HandlePluginTunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error {
	if err := s.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	if eof {
		return s.CloseTunnel(sessionID, tunnelID)
	}
	data, err := decodeTunnelData(dataBase64)
	if err != nil {
		return err
	}
	return s.RouteTunnelFrameFromPlugin(ctx, sessionID, tunnelID, data)
}

// HandlePluginTunnelClose closes a tunnel for the session.
func (s *EmbedTunnelService) HandlePluginTunnelClose(_ context.Context, pluginID, sessionID, tunnelID string) error {
	if err := s.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	return s.CloseTunnel(sessionID, tunnelID)
}

// HandlePluginReportLocalEmbed rejects Mode B in v1 broker-default builds.
func (s *EmbedTunnelService) HandlePluginReportLocalEmbed(_ context.Context, pluginID, sessionID string, _ json.RawMessage) error {
	if err := s.assertPluginSession(pluginID, sessionID); err != nil {
		return err
	}
	return ErrLocalEmbedNotSupported
}

func (s *EmbedTunnelService) assertPluginSession(pluginID, sessionID string) error {
	if s == nil || s.registry == nil {
		return domain.ErrSessionNotFound
	}
	entry, ok := s.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return domain.ErrSessionNotFound
	}
	return nil
}

var _ PluginEmbedSink = (*EmbedTunnelService)(nil)
