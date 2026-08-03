package ipc

import "context"

// At credit 0 every purpose behaves the same way: the backend's own upstream read loop stops
// pulling more data until credit frees up, so backpressure propagates to the real source (SSH
// stdout pipe, relayed socket, browser input stream) instead of an in-process buffer growing
// without bound.
//
// embed-stream used to be the exception, dropping the oldest unsent frame at credit 0
// (latest-frame-wins). That was written for video, where a frame is self-contained and losing
// an old one costs a dropped picture. The purpose carries no such thing: its outbound direction
// is browser control input, where every event is a state transition, not a snapshot. A dropped
// KeyEvent with down=0 leaves the key held down on the remote machine — a stuck Ctrl or Shift,
// with nothing in any log. Its inbound direction is an incremental framebuffer, which does not
// survive a lost delta either. There is no direction in which dropping was correct.
//
// udp-relay looks like a candidate for dropping but is not: at credit 0 its backend stops
// reading the socket and the OS receive buffer bounds and drops excess datagrams, which is
// correct UDP behavior arrived at without a host-side policy.
//
// If a genuine video transport ever needs latest-frame-wins, it belongs to a purpose that opts
// into it explicitly, not as the default for a channel carrying user input.

// backendGate is the capacity signal a purpose backend's read loop
// blocks on before pulling more data from its upstream source. It never itself queues
// anything — it only reports "credit is available", derived from the channel's live
// outbound credit — so no unbounded in-process buffer can grow behind it.
type backendGate struct {
	credit *channelCredit
}

func newBackendGate(credit *channelCredit) *backendGate {
	return &backendGate{credit: credit}
}

// WaitForCapacity blocks until outbound credit is available, or ctx is done. It does not
// consume credit: consumption happens at actual send time (channel.Send), keeping this a
// pure "may I proceed" signal.
func (g *backendGate) WaitForCapacity(ctx context.Context) error {
	return g.credit.WaitOutboundAvailable(ctx)
}
