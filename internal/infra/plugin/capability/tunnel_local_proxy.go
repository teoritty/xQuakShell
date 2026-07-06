package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TunnelLocalProxy enforces localConnId ownership for pre-bind local tunnel RPC.
type TunnelLocalProxy struct {
	pluginID string
	inbound  domainplugin.TunnelInboundPort
	dial     *TunnelDialProxy
	mu       sync.Mutex
	locals   map[string]struct{}
}

// NewTunnelLocalProxy creates a tunnel local-side RPC proxy.
func NewTunnelLocalProxy(pluginID string, inbound domainplugin.TunnelInboundPort, dial *TunnelDialProxy) *TunnelLocalProxy {
	return &TunnelLocalProxy{
		pluginID: pluginID,
		inbound:  inbound,
		dial:     dial,
		locals:   make(map[string]struct{}),
	}
}

type tunnelLocalParams struct {
	LocalConnID string `json:"localConnId"`
}

type tunnelBindParams struct {
	LocalConnID string `json:"localConnId"`
	TunnelID    string `json:"tunnelId"`
}

// RegisterLocal records a localConnId issued to this plugin via tunnel.localAccept.
func (p *TunnelLocalProxy) RegisterLocal(localConnID string) {
	if p == nil {
		return
	}
	localConnID = strings.TrimSpace(localConnID)
	if localConnID == "" {
		return
	}
	p.mu.Lock()
	p.locals[localConnID] = struct{}{}
	p.mu.Unlock()
}

// ReleaseLocal removes a localConnId after close notification or successful plugin RPC.
func (p *TunnelLocalProxy) ReleaseLocal(localConnID string) {
	if p == nil {
		return
	}
	localConnID = strings.TrimSpace(localConnID)
	if localConnID == "" {
		return
	}
	p.mu.Lock()
	delete(p.locals, localConnID)
	p.mu.Unlock()
}

// CloseAll clears every owned local handle (process shutdown).
func (p *TunnelLocalProxy) CloseAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.locals = make(map[string]struct{})
}

// LocalWrite handles tunnel.localWrite.
func (p *TunnelLocalProxy) LocalWrite(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	localConnID, err := parseLocalConnID(params)
	if err != nil {
		return nil, err
	}
	if err := p.requireLocal(localConnID); err != nil {
		return nil, err
	}
	return p.inbound.TunnelLocalWrite(ctx, p.pluginID, params)
}

// LocalClose handles tunnel.localClose from the plugin.
func (p *TunnelLocalProxy) LocalClose(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	localConnID, err := parseLocalConnID(params)
	if err != nil {
		return nil, err
	}
	if err := p.requireLocal(localConnID); err != nil {
		return nil, err
	}

	result, err := p.inbound.TunnelLocalClose(ctx, p.pluginID, params)
	if err != nil {
		return nil, err
	}
	p.ReleaseLocal(localConnID)
	return result, nil
}

// Bind handles tunnel.bind. The dial-proxy channel slot stays reserved until the SSH channel closes.
func (p *TunnelLocalProxy) Bind(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.inbound == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	var req tunnelBindParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid tunnel.bind params: %w", err)
	}
	localConnID := strings.TrimSpace(req.LocalConnID)
	tunnelID := strings.TrimSpace(req.TunnelID)
	if localConnID == "" || tunnelID == "" {
		return nil, fmt.Errorf("invalid tunnel.bind params: localConnId and tunnelId required")
	}
	if err := p.requireLocal(localConnID); err != nil {
		return nil, err
	}
	if p.dial == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if err := p.dial.requireTunnel(tunnelID); err != nil {
		return nil, err
	}

	result, err := p.inbound.TunnelBind(ctx, p.pluginID, params)
	if err != nil {
		return nil, err
	}
	p.ReleaseLocal(localConnID)
	return result, nil
}

func parseLocalConnID(params json.RawMessage) (string, error) {
	var req tunnelLocalParams
	if err := json.Unmarshal(params, &req); err != nil {
		return "", fmt.Errorf("invalid tunnel local params: %w", err)
	}
	localConnID := strings.TrimSpace(req.LocalConnID)
	if localConnID == "" {
		return "", fmt.Errorf("invalid tunnel local params: localConnId required")
	}
	return localConnID, nil
}

func (p *TunnelLocalProxy) requireLocal(localConnID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.locals[localConnID]; !ok {
		return domainplugin.ErrHandleNotFound
	}
	return nil
}
