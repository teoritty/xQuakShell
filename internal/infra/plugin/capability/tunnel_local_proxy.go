package capability

import (
	"context"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TunnelLocalProxy forwards pre-bind local tunnel RPC to the usecase inbound port.
type TunnelLocalProxy struct {
	pluginID string
	inbound  domainplugin.TunnelInboundPort
	onBind   func()
}

// NewTunnelLocalProxy creates a tunnel local-side RPC proxy.
func NewTunnelLocalProxy(pluginID string, inbound domainplugin.TunnelInboundPort, onBind func()) *TunnelLocalProxy {
	return &TunnelLocalProxy{pluginID: pluginID, inbound: inbound, onBind: onBind}
}

// LocalWrite handles tunnel.localWrite.
func (p *TunnelLocalProxy) LocalWrite(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return p.inbound.TunnelLocalWrite(ctx, p.pluginID, params)
}

// LocalClose handles tunnel.localClose from the plugin.
func (p *TunnelLocalProxy) LocalClose(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return p.inbound.TunnelLocalClose(ctx, p.pluginID, params)
}

// Bind handles tunnel.bind.
func (p *TunnelLocalProxy) Bind(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	result, err := p.inbound.TunnelBind(ctx, p.pluginID, params)
	if err == nil && p.onBind != nil {
		p.onBind()
	}
	return result, err
}
