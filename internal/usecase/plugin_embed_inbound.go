package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginEmbedSink receives embed-related plugin RPC from the session layer.
type PluginEmbedSink interface {
	HandlePluginRegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error)
	HandlePluginTunnelOpen(ctx context.Context, pluginID, sessionID, tunnelID string) error
	HandlePluginTunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error
	HandlePluginTunnelClose(ctx context.Context, pluginID, sessionID, tunnelID string) error
	HandlePluginReportLocalEmbed(ctx context.Context, pluginID, sessionID string, params json.RawMessage) error
}

// PluginEmbedInbound adapts plugin embed RPC to the session manager.
type PluginEmbedInbound struct {
	mu      sync.RWMutex
	handler PluginEmbedSink
}

// NewPluginEmbedInbound creates an embed inbound adapter.
func NewPluginEmbedInbound() *PluginEmbedInbound {
	return &PluginEmbedInbound{}
}

// SetHandler binds the session manager after composition.
func (p *PluginEmbedInbound) SetHandler(h PluginEmbedSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

func (p *PluginEmbedInbound) handlerOrErr() (PluginEmbedSink, error) {
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()
	if h == nil {
		return nil, domainplugin.ErrPluginNotRunning
	}
	return h, nil
}

// RegisterEmbed implements embed registration from plugins.
func (p *PluginEmbedInbound) RegisterEmbed(ctx context.Context, pluginID, sessionID, uiEntry string, tunnelIDs []string) (json.RawMessage, error) {
	h, err := p.handlerOrErr()
	if err != nil {
		return nil, err
	}
	return h.HandlePluginRegisterEmbed(ctx, pluginID, sessionID, uiEntry, tunnelIDs)
}

// TunnelOpen marks a tunnel ready for a session.
func (p *PluginEmbedInbound) TunnelOpen(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	h, err := p.handlerOrErr()
	if err != nil {
		return err
	}
	return h.HandlePluginTunnelOpen(ctx, pluginID, sessionID, tunnelID)
}

// TunnelFrame forwards opaque bytes from plugin to browser.
func (p *PluginEmbedInbound) TunnelFrame(ctx context.Context, pluginID, sessionID, tunnelID, dataBase64 string, eof bool) error {
	h, err := p.handlerOrErr()
	if err != nil {
		return err
	}
	return h.HandlePluginTunnelFrame(ctx, pluginID, sessionID, tunnelID, dataBase64, eof)
}

// TunnelClose closes a tunnel for a session.
func (p *PluginEmbedInbound) TunnelClose(ctx context.Context, pluginID, sessionID, tunnelID string) error {
	h, err := p.handlerOrErr()
	if err != nil {
		return err
	}
	return h.HandlePluginTunnelClose(ctx, pluginID, sessionID, tunnelID)
}

// ReportLocalEmbed records Mode B local server metadata.
func (p *PluginEmbedInbound) ReportLocalEmbed(ctx context.Context, pluginID, sessionID string, params json.RawMessage) error {
	h, err := p.handlerOrErr()
	if err != nil {
		return err
	}
	return h.HandlePluginReportLocalEmbed(ctx, pluginID, sessionID, params)
}

// NormalizeTunnelIDs returns default tunnel IDs when empty.
func NormalizeTunnelIDs(ids []string) []string {
	if len(ids) == 0 {
		return []string{"main"}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return []string{"main"}
	}
	return out
}
