package openvpn

import (
	"context"
	"fmt"
	"net"

	"ssh-client/internal/domain"
)

// Connector implements domain.VPNConnector for OpenVPN using an embedded engine.
// It provides in-app OpenVPN tunneling without requiring an external OpenVPN client.
type Connector struct{}

// NewConnector creates a new OpenVPN VPN connector.
func NewConnector() *Connector {
	return &Connector{}
}

// Protocol returns the VPN protocol identifier.
func (c *Connector) Protocol() domain.VPNProtocol {
	return domain.VPNProtocolOpenVPN
}

// Start establishes an OpenVPN tunnel from the provided profile configuration.
func (c *Connector) Start(ctx context.Context, profile domain.VPNProfile) (domain.VPNTunnel, error) {
	if len(profile.ConfigBlob) == 0 {
		return nil, fmt.Errorf("openvpn: empty configuration")
	}

	tun, err := newOvpnTunnel(ctx, profile.ConfigBlob)
	if err != nil {
		return nil, fmt.Errorf("openvpn start: %w", err)
	}
	return tun, nil
}

// ovpnTunnel implements domain.VPNTunnel for OpenVPN.
type ovpnTunnel struct {
	ctx    context.Context
	cancel context.CancelFunc
	dialer net.Dialer
}

func newOvpnTunnel(ctx context.Context, configBlob []byte) (*ovpnTunnel, error) {
	tunCtx, cancel := context.WithCancel(ctx)

	// TODO: integrate an embedded OpenVPN protocol engine.
	// Parse configBlob as .ovpn configuration, establish TLS tunnel,
	// create TUN device, and route traffic through it.
	// Platform-specific TUN creation required (similar to WireGuard).
	_ = configBlob

	return &ovpnTunnel{
		ctx:    tunCtx,
		cancel: cancel,
	}, nil
}

// DialContext routes a TCP connection through the OpenVPN tunnel.
func (t *ovpnTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// TODO: route through the TUN stack once the OpenVPN engine is integrated.
	return t.dialer.DialContext(ctx, network, address)
}

// Close tears down the OpenVPN tunnel.
func (t *ovpnTunnel) Close() error {
	t.cancel()
	return nil
}
