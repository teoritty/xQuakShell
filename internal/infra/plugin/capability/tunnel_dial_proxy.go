package capability

import (
	"context"
	"encoding/json"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TunnelDialProxy enforces per-plugin channel limits for tunnel.dial / tunnel.close.
type TunnelDialProxy struct {
	pluginID string
	inbound  domainplugin.TunnelInboundPort
	max      int
	mu       sync.Mutex
	open     int
}

// NewTunnelDialProxy creates a tunnel dial proxy with a channel limit from manifest (0 = default).
func NewTunnelDialProxy(pluginID string, caps *domainplugin.TunnelCaps, inbound domainplugin.TunnelInboundPort) *TunnelDialProxy {
	max := domainplugin.DefaultMaxTunnelChannels
	if caps != nil && caps.MaxConcurrentChannels > 0 {
		max = caps.MaxConcurrentChannels
	}
	return &TunnelDialProxy{pluginID: pluginID, inbound: inbound, max: max}
}

// Dial forwards tunnel.dial when under the channel limit.
func (p *TunnelDialProxy) Dial(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	p.mu.Lock()
	if p.open >= p.max {
		p.mu.Unlock()
		return nil, domainplugin.ErrRateLimited
	}
	p.open++
	p.mu.Unlock()

	result, err := p.inbound.TunnelDial(ctx, p.pluginID, params)
	if err != nil {
		p.mu.Lock()
		if p.open > 0 {
			p.open--
		}
		p.mu.Unlock()
		return nil, err
	}
	return result, nil
}

// ReleaseSlot decrements the open channel count after tunnel.bind transfers ownership.
func (p *TunnelDialProxy) ReleaseSlot() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.open > 0 {
		p.open--
	}
	p.mu.Unlock()
}
func (p *TunnelDialProxy) Close(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	result, err := p.inbound.TunnelClose(ctx, p.pluginID, params)
	if err == nil {
		p.mu.Lock()
		if p.open > 0 {
			p.open--
		}
		p.mu.Unlock()
	}
	return result, err
}
