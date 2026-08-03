package pinger

import (
	"context"
	"log/slog"
	"net"
	"time"

	"xquakshell/internal/domain"
)

const defaultPingTimeout = 3 * time.Second

// TCPPinger performs TCP connect probes via net.Dialer.
type TCPPinger struct {
	timeout time.Duration
}

// NewTCPPinger creates a TCPPinger with the given dial timeout.
// If timeout <= 0, defaultPingTimeout is used.
func NewTCPPinger(timeout time.Duration) *TCPPinger {
	if timeout <= 0 {
		timeout = defaultPingTimeout
	}
	return &TCPPinger{timeout: timeout}
}

var _ domain.Pinger = (*TCPPinger)(nil)

// Ping dials network/address and closes the connection on success.
func (p *TCPPinger) Ping(ctx context.Context, network, address string) error {
	d := net.Dialer{Timeout: p.timeout}
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return err
	}
	if closeErr := conn.Close(); closeErr != nil {
		slog.Warn("ping: failed to close tcp conn", "addr", address, "err", closeErr)
	}
	return nil
}
