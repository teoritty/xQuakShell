package ipc

import (
	"context"
	"errors"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ErrUnknownChannelPurpose reports a purpose the host has no credit policy for. Such a channel
// would be born with a zero window: its first Send would block forever and its first inbound
// frame would be a protocol violation, killing a plugin that did nothing wrong.
var ErrUnknownChannelPurpose = errors.New("ipc: no initial credit defined for channel purpose")

// channelDataPath adapts one ipc.channel to the domain's ChannelDataPath contract. It exists so
// purpose backends can move bytes without importing the frame layer, and it is deliberately a
// separate type from channel: channel changes when the bus's internals change, this changes
// when the domain contract does.
type channelDataPath struct {
	ch   *channel
	conn *Conn
}

// OpenDataPath registers a flow-controlled channel for an already-authorized id and returns its
// domain-facing data path. It is the composition root's single entry point into the bus, and
// the reason a ChannelHandle can no longer be built without one.
func (c *Conn) OpenDataPath(id uint32, purpose string) (domainplugin.ChannelDataPath, error) {
	if domainplugin.InitialCredit(purpose) <= 0 {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannelPurpose, purpose)
	}
	ch := c.mux.Register(id, purpose, c.channelThroughputKbps)
	// Hold the channel's outbound emission until the channel.open reply reaches the wire
	// (Conn.MarkOpened): a host->plugin frame that arrives before the plugin has registered this
	// channelId is fatal to the plugin. Engaged here, before the backend is wired, so no send
	// can race it.
	ch.gateUntilOpened()
	return &channelDataPath{ch: ch, conn: c}, nil
}

// MarkChannelOpened releases a channel's outbound send gate (see channel.opened). Conn calls it
// from handleIncomingRequest once the channel.open reply is on the wire; it is exported so a
// caller that drives ChannelProxy.Open outside the inbound-request path (the composition-root
// seam tests) can stand in for that step. A no-op for an unknown or un-gated id.
func (c *Conn) MarkChannelOpened(id uint32) {
	c.mux.MarkOpened(id)
}

// Send emits one outbound kind=0x02 frame, blocking per the channel's exhaustion policy.
func (p *channelDataPath) Send(ctx context.Context, payload []byte) error {
	return p.ch.Send(ctx, payload)
}

// Recv returns the next inbound data payload, or ok=false once the channel is closed and
// drained. It returns the frame only: credit for it is returned by Ack, once the caller has
// actually handed it on.
func (p *channelDataPath) Recv() ([]byte, bool) {
	f, ok := p.ch.Recv()
	if !ok {
		return nil, false
	}
	return f.Payload, true
}

// WaitForCapacity blocks until outbound credit exists, without consuming it.
func (p *channelDataPath) WaitForCapacity(ctx context.Context) error {
	return p.ch.Gate().WaitForCapacity(ctx)
}

// Ack returns one credit unit for the frame the caller has just passed to its consumer.
//
// The local grant precedes the wire notification on purpose (see channel.grantInbound). A
// failed emission is not fatal: the plugin simply does not learn of the credit yet, which is
// the conservative direction, and a connection sick enough to fail this write is already being
// torn down elsewhere.
func (p *channelDataPath) Ack(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if p.ch.Closed() {
		return nil
	}
	p.ch.grantInbound(1)
	if err := p.conn.WriteCredit(p.ch.id, 1); err != nil {
		// A closed/failed conn means teardown is already underway; the plugin will never
		// need this credit. Surfacing it would only make every backend's pump log noise on
		// an ordinary shutdown race.
		return nil
	}
	return nil
}

// Close releases the channel and unblocks anything parked on it. Idempotent.
//
// The channel is not dropped from the bus immediately — see channelMux.CloseAndRelease for why
// frames the plugin already sent for this id must land as no-ops rather than kill it.
func (p *channelDataPath) Close() error {
	p.conn.mux.CloseAndRelease(p.ch.id)
	return nil
}

var _ domainplugin.ChannelDataPath = (*channelDataPath)(nil)
var _ domainplugin.ChannelDataPathOpener = (*Conn)(nil)
