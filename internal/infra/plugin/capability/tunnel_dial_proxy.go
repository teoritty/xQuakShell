package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TunnelDialProxy enforces per-plugin channel limits and tunnelId ownership for tunnel.dial / tunnel.close.
type TunnelDialProxy struct {
	pluginID     string
	inbound      domainplugin.TunnelInboundPort
	max          int
	mu           sync.Mutex
	tunnels      map[string]struct{}
	pendingDials int
}

// NewTunnelDialProxy creates a tunnel dial proxy with a channel limit from manifest (0 = default).
func NewTunnelDialProxy(pluginID string, caps *domainplugin.TunnelCaps, inbound domainplugin.TunnelInboundPort) *TunnelDialProxy {
	max := domainplugin.DefaultMaxTunnelChannels
	if caps != nil && caps.MaxConcurrentChannels > 0 {
		max = caps.MaxConcurrentChannels
	}
	return &TunnelDialProxy{
		pluginID: pluginID,
		inbound:  inbound,
		max:      max,
		tunnels:  make(map[string]struct{}),
	}
}

type tunnelDialResult struct {
	TunnelID string `json:"tunnelId"`
}

type tunnelCloseParams struct {
	TunnelID string `json:"tunnelId"`
}

// Dial forwards tunnel.dial when under the channel limit and registers the returned tunnelId.
func (p *TunnelDialProxy) Dial(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}

	p.mu.Lock()
	if len(p.tunnels)+p.pendingDials >= p.max {
		p.mu.Unlock()
		return nil, domainplugin.ErrRateLimited
	}
	p.pendingDials++
	committed := false
	p.mu.Unlock()
	defer func() {
		if !committed {
			p.mu.Lock()
			p.pendingDials--
			p.mu.Unlock()
		}
	}()

	result, err := p.inbound.TunnelDial(ctx, p.pluginID, params)
	if err != nil {
		return nil, err
	}

	var dialRes tunnelDialResult
	if err := json.Unmarshal(result, &dialRes); err != nil {
		return nil, fmt.Errorf("invalid tunnel.dial result: %w", err)
	}
	tunnelID := strings.TrimSpace(dialRes.TunnelID)
	if tunnelID == "" {
		return nil, fmt.Errorf("invalid tunnel.dial result: tunnelId required")
	}

	p.mu.Lock()
	p.pendingDials--
	p.tunnels[tunnelID] = struct{}{}
	committed = true
	p.mu.Unlock()

	return result, nil
}

// Close closes a tunnel channel owned by this plugin process.
func (p *TunnelDialProxy) Close(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	var req tunnelCloseParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid tunnel.close params: %w", err)
	}
	tunnelID := strings.TrimSpace(req.TunnelID)
	if tunnelID == "" {
		return nil, fmt.Errorf("invalid tunnel.close params: tunnelId required")
	}
	if err := p.requireTunnel(tunnelID); err != nil {
		return nil, err
	}

	result, err := p.inbound.TunnelClose(ctx, p.pluginID, params)
	if err != nil {
		return nil, err
	}
	p.takeTunnel(tunnelID)
	return result, nil
}

// ReleaseTunnel removes a tunnelId from this process after bind or host-initiated timeout.
func (p *TunnelDialProxy) ReleaseTunnel(tunnelID string) {
	if p == nil {
		return
	}
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return
	}
	p.takeTunnel(tunnelID)
}

// CloseAll clears every owned tunnel handle (process shutdown).
func (p *TunnelDialProxy) CloseAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunnels = make(map[string]struct{})
	p.pendingDials = 0
}

func (p *TunnelDialProxy) requireTunnel(tunnelID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.tunnels[tunnelID]; !ok {
		return domainplugin.ErrHandleNotFound
	}
	return nil
}

func (p *TunnelDialProxy) takeTunnel(tunnelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tunnels, tunnelID)
}
