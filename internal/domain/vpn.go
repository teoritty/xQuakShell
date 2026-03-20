package domain

import (
	"context"
	"net"
)

// VPNProtocol identifies the VPN tunneling protocol.
type VPNProtocol string

const (
	VPNProtocolWireGuard VPNProtocol = "wireguard"
	VPNProtocolOpenVPN   VPNProtocol = "openvpn"
)

// VPNProfile stores the configuration for an optional VPN tunnel associated with a connection.
type VPNProfile struct {
	ID       string      `json:"id"`
	Label    string      `json:"label"`
	Protocol VPNProtocol `json:"protocol"`
	// ConfigBlob holds the raw VPN configuration bytes (e.g., WireGuard conf, OpenVPN ovpn)
	// stored encrypted inside vault.age.
	ConfigBlob []byte `json:"configBlob"`
}

// VPNTunnel represents an active VPN tunnel that can provide routed connections.
type VPNTunnel interface {
	// DialContext returns a net.Conn routed through the VPN tunnel to the given address.
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	// Close tears down the tunnel and releases all resources.
	Close() error
}

// VPNConnector creates and manages VPN tunnels.
type VPNConnector interface {
	// Protocol returns the VPN protocol this connector handles.
	Protocol() VPNProtocol
	// Start establishes the VPN tunnel using the given profile configuration.
	Start(ctx context.Context, profile VPNProfile) (VPNTunnel, error)
}
