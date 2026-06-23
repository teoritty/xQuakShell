package usecase

import (
	"context"
	"encoding/json"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginViewSink receives inbound view messages from plugins.
type PluginViewSink interface {
	HandlePluginViewMessage(pluginID, panelID string, message json.RawMessage) error
}

// PluginViewInbound adapts plugin view IPC to the presentation layer.
type PluginViewInbound struct {
	mu       sync.RWMutex
	handler  PluginViewSink
	registry *PluginRegistry
}

// NewPluginViewInbound creates a view inbound adapter.
func NewPluginViewInbound(registry *PluginRegistry) *PluginViewInbound {
	return &PluginViewInbound{registry: registry}
}

// SetHandler binds the Wails event emitter after composition.
func (p *PluginViewInbound) SetHandler(h PluginViewSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

// PostMessage implements domainplugin.ViewInboundPort.
func (p *PluginViewInbound) PostMessage(_ context.Context, pluginID, panelID string, message json.RawMessage) error {
	if p.registry != nil {
		if _, err := p.registry.ViewEntry(pluginID, panelID); err != nil {
			return domainplugin.ErrCapabilityDenied
		}
	}
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()
	if h == nil {
		return domainplugin.ErrPluginNotRunning
	}
	return h.HandlePluginViewMessage(pluginID, panelID, message)
}

var _ domainplugin.ViewInboundPort = (*PluginViewInbound)(nil)
