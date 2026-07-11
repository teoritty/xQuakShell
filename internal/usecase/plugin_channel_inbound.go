package usecase

import (
	"context"
	"encoding/json"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ChannelSink receives channel.open/channel.close plugin RPC from the session layer. The
// concrete implementation is the per-plugin-process capability.ChannelProxy, wired at
// composition root, mirroring how PluginEmbedSink is bound to the session manager.
type ChannelSink interface {
	HandlePluginChannelOpen(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	HandlePluginChannelClose(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
}

// PluginChannelInbound adapts plugin channel.open/channel.close RPC to the bound ChannelSink,
// mirroring PluginEmbedInbound's late-bound structure exactly.
type PluginChannelInbound struct {
	mu      sync.RWMutex
	handler ChannelSink
}

// NewPluginChannelInbound creates a channel inbound adapter.
func NewPluginChannelInbound() *PluginChannelInbound {
	return &PluginChannelInbound{}
}

// SetHandler binds the channel sink after composition.
func (p *PluginChannelInbound) SetHandler(h ChannelSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

func (p *PluginChannelInbound) handlerOrErr() (ChannelSink, error) {
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()
	if h == nil {
		return nil, domainplugin.ErrPluginNotRunning
	}
	return h, nil
}

// Open implements domainplugin.ChannelInboundPort.
func (p *PluginChannelInbound) Open(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h, err := p.handlerOrErr()
	if err != nil {
		return nil, err
	}
	return h.HandlePluginChannelOpen(ctx, pluginID, params)
}

// Close implements domainplugin.ChannelInboundPort.
func (p *PluginChannelInbound) Close(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h, err := p.handlerOrErr()
	if err != nil {
		return nil, err
	}
	return h.HandlePluginChannelClose(ctx, pluginID, params)
}

var _ domainplugin.ChannelInboundPort = (*PluginChannelInbound)(nil)
