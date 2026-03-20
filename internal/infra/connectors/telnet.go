package connectors

import (
	"context"
	"fmt"
	"net"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/stream"
)

// TelnetConnector establishes a Telnet session via TCP and streams I/O to the terminal.
type TelnetConnector struct{}

// NewTelnetConnector creates a Telnet connector.
func NewTelnetConnector() *TelnetConnector {
	return &TelnetConnector{}
}

// Protocol implements domain.SessionConnector.
func (c *TelnetConnector) Protocol() string {
	return domain.ProtocolTelnet
}

// Connect implements domain.SessionConnector.
func (c *TelnetConnector) Connect(ctx context.Context, conn *domain.Connection, deps domain.ConnectorDeps, hooks domain.ConnectorHooks) error {
	if conn.TelnetConfig == nil || conn.TelnetConfig.Host == "" {
		hooks.UpdateState(domain.SessionError, "telnet host not configured")
		return nil
	}

	host := conn.TelnetConfig.Host
	port := conn.TelnetConfig.Port
	if port <= 0 {
		port = 23
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	timeoutSec := 15
	if data, err := deps.VaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.ConnectionTimeoutSec > 0 {
		timeoutSec = data.Settings.Transfer.ConnectionTimeoutSec
	}
	dialer := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}

	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		hooks.UpdateState(domain.SessionError, fmt.Sprintf("telnet connect: %s", err.Error()))
		return nil
	}

	bridge := stream.NewBridge()
	outputCh, err := bridge.StartFromConn(ctx, netConn)
	if err != nil {
		netConn.Close()
		hooks.UpdateState(domain.SessionError, fmt.Sprintf("telnet stream: %s", err.Error()))
		return nil
	}

	hooks.SetPTYBridge(bridge)
	if hooks.OnStreamReady != nil {
		hooks.OnStreamReady(outputCh)
	}
	hooks.UpdateState(domain.SessionReady, "")
	return nil
}
