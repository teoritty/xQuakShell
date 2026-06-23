package usecase

import (
	"context"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginSessionSink receives inbound session RPC from plugins.
type PluginSessionSink interface {
	HandlePluginUpdateState(pluginID, sessionID, state, errMsg string) error
	HandlePluginWriteTerminal(pluginID, sessionID string, data []byte) error
}

// PluginSessionInbound adapts plugin IPC to the session manager (breaks init cycles).
type PluginSessionInbound struct {
	mu      sync.RWMutex
	handler PluginSessionSink
}

// NewPluginSessionInbound creates an inbound adapter with a late-bound handler.
func NewPluginSessionInbound() *PluginSessionInbound {
	return &PluginSessionInbound{}
}

// SetHandler binds the session manager after composition.
func (p *PluginSessionInbound) SetHandler(h PluginSessionSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

// UpdateState implements domainplugin.SessionInboundPort.
func (p *PluginSessionInbound) UpdateState(_ context.Context, pluginID, sessionID, state, errMsg string) error {
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()
	if h == nil {
		return domainplugin.ErrPluginNotRunning
	}
	return h.HandlePluginUpdateState(pluginID, sessionID, state, errMsg)
}

// WriteTerminal implements domainplugin.SessionInboundPort.
func (p *PluginSessionInbound) WriteTerminal(_ context.Context, pluginID, sessionID, dataBase64 string) error {
	p.mu.RLock()
	h := p.handler
	p.mu.RUnlock()
	if h == nil {
		return domainplugin.ErrPluginNotRunning
	}
	data, err := decodeTerminalPayload(dataBase64)
	if err != nil {
		return err
	}
	return h.HandlePluginWriteTerminal(pluginID, sessionID, data)
}

var _ domainplugin.SessionInboundPort = (*PluginSessionInbound)(nil)
