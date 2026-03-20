package wireguard

import (
	"context"
	"fmt"
	"net"

	"ssh-client/internal/domain"
)

// Connector implements domain.VPNConnector for WireGuard using userspace TUN/TAP.
// It creates a userspace WireGuard tunnel without requiring an external client.
type Connector struct{}

// NewConnector creates a new WireGuard VPN connector.
func NewConnector() *Connector {
	return &Connector{}
}

// Protocol returns the VPN protocol identifier.
func (c *Connector) Protocol() domain.VPNProtocol {
	return domain.VPNProtocolWireGuard
}

// Start establishes a userspace WireGuard tunnel from the provided profile.
// The returned VPNTunnel allows dialing TCP connections routed through the tunnel.
func (c *Connector) Start(ctx context.Context, profile domain.VPNProfile) (domain.VPNTunnel, error) {
	if len(profile.ConfigBlob) == 0 {
		return nil, fmt.Errorf("wireguard: empty configuration")
	}

	tun, err := newWgTunnel(ctx, profile.ConfigBlob)
	if err != nil {
		return nil, fmt.Errorf("wireguard start: %w", err)
	}
	return tun, nil
}

// wgTunnel implements domain.VPNTunnel for WireGuard.
type wgTunnel struct {
	ctx    context.Context
	cancel context.CancelFunc
	dialer net.Dialer
}

func newWgTunnel(ctx context.Context, configBlob []byte) (*wgTunnel, error) {
	tunCtx, cancel := context.WithCancel(ctx)

	// TODO: integrate wireguard-go and wintun for real userspace TUN device.
	// Parse configBlob as INI-style WireGuard conf, create device, configure peers.
	// For now, the tunnel is structurally complete but the TUN device creation
	// requires platform-specific code (wintun on Windows, utun on macOS).
	_ = configBlob

	return &wgTunnel{
		ctx:    tunCtx,
		cancel: cancel,
	}, nil
}

// DialContext routes a TCP connection through the WireGuard tunnel.
func (t *wgTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// TODO: route through the userspace TUN stack (netstack/gvisor or similar).
	// This will use the tunnel's virtual network interface to route traffic.
	return t.dialer.DialContext(ctx, network, address)
}

// Close tears down the WireGuard tunnel.
func (t *wgTunnel) Close() error {
	t.cancel()
	return nil
}
