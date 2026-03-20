package vpn

import (
	"context"
	"fmt"
	"sync"

	"ssh-client/internal/domain"
)

// Registry manages VPN connectors and active tunnels.
type Registry struct {
	mu         sync.Mutex
	connectors map[domain.VPNProtocol]domain.VPNConnector
	active     map[string]domain.VPNTunnel
}

// NewRegistry creates a VPN registry with the given connectors.
func NewRegistry(connectors ...domain.VPNConnector) *Registry {
	r := &Registry{
		connectors: make(map[domain.VPNProtocol]domain.VPNConnector),
		active:     make(map[string]domain.VPNTunnel),
	}
	for _, c := range connectors {
		r.connectors[c.Protocol()] = c
	}
	return r
}

// StartTunnel creates and starts a VPN tunnel for the given profile.
// The tunnel is tracked by profileID and can be stopped later.
func (r *Registry) StartTunnel(ctx context.Context, profile domain.VPNProfile) (domain.VPNTunnel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tun, ok := r.active[profile.ID]; ok {
		return tun, nil
	}

	connector, ok := r.connectors[profile.Protocol]
	if !ok {
		return nil, fmt.Errorf("no VPN connector for protocol %s", profile.Protocol)
	}

	tun, err := connector.Start(ctx, profile)
	if err != nil {
		return nil, err
	}

	r.active[profile.ID] = tun
	return tun, nil
}

// StopTunnel tears down the tunnel for the given profile ID.
func (r *Registry) StopTunnel(profileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tun, ok := r.active[profileID]
	if !ok {
		return nil
	}

	delete(r.active, profileID)
	return tun.Close()
}

// StopAll tears down all active tunnels.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, tun := range r.active {
		tun.Close()
		delete(r.active, id)
	}
}

// GetTunnel returns the active tunnel for a profile, if any.
func (r *Registry) GetTunnel(profileID string) (domain.VPNTunnel, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tun, ok := r.active[profileID]
	return tun, ok
}
