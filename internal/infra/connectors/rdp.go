package connectors

import (
	"context"

	"ssh-client/internal/domain"
)

// RDPConnector marks RDP sessions as ready.
// The actual RDP connection (via guacd gateway) is initiated by the frontend
// when it calls RDPStart, which returns a WebSocket URL for guacamole-common-js.
type RDPConnector struct{}

// NewRDPConnector creates an RDP connector.
func NewRDPConnector() *RDPConnector {
	return &RDPConnector{}
}

// Protocol implements domain.SessionConnector.
func (c *RDPConnector) Protocol() string {
	return domain.ProtocolRDP
}

// Connect implements domain.SessionConnector.
// For RDP the session is marked ready immediately; the guacd connection
// happens later when the frontend requests a WebSocket URL via RDPStart.
func (c *RDPConnector) Connect(_ context.Context, conn *domain.Connection, _ domain.ConnectorDeps, hooks domain.ConnectorHooks) error {
	if conn.RDPConfig == nil || conn.RDPConfig.Host == "" {
		hooks.UpdateState(domain.SessionError, "rdp host not configured")
		return nil
	}
	hooks.UpdateState(domain.SessionReady, "")
	return nil
}
