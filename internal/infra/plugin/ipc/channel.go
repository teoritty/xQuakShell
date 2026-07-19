package ipc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// channelState is the ADR-011 §Session lifecycle coupling state machine for one channel.
type channelState int

const (
	channelStateOpen channelState = iota
	channelStateClosed
)

// channelFrame is one inbound kind=0x02/0x03 frame queued for a channel's consumer.
type channelFrame struct {
	Kind    byte
	Payload []byte
}

// channel holds one channelId's lifecycle state and inbound queue. Stage 2 deliberately
// has no credit/flow-control policy (Stage 5) — the queue is unbounded so a slow consumer
// on one channel cannot block delivery to another channel or to the JSON-RPC control plane,
// which share the single-threaded read loop that feeds all channels' deliver calls.
type channel struct {
	id uint32

	mu     sync.Mutex
	state  channelState
	queue  []channelFrame
	notify chan struct{}

	closeOnce sync.Once
	closed    chan struct{}

	// opened gates the first host->plugin send until the channel.open reply has been written to
	// the wire (Conn.MarkOpened, called right after the reply). A plugin registers the channelId
	// only once its channel.open Call returns, so a kind=0x02 data frame that reaches it before
	// the reply is a frame for a channelId it does not yet know is open — a fatal protocol
	// violation on the plugin side (observed with a VNC server, which speaks first: the host's
	// relay pump read the RFB banner and sent it before the reply). Inbound (plugin->host)
	// delivery is unaffected; only the host's own emission waits.
	//
	// nil means "not gated" — the default, so a channel constructed directly (unit tests, or any
	// caller not driving the real channel.open RPC) is immediately sendable. Only OpenDataPath,
	// the production seam reached through an actual channel.open, engages the gate via
	// gateUntilOpened before the backend is wired.
	opened   chan struct{}
	openOnce sync.Once

	// Flow control (ADR-011 §2b). Always populated: newFlowChannel is the only constructor
	// reachable from channelMux.Register, so there is no such thing as a live channel without
	// a credit window.
	purpose    string
	credit     *channelCredit
	throttle   *tokenBucket
	gate       *backendGate
	writeFrame func(payload []byte) error
	// maxOutbound caps how large kind=0x03 grants may grow the outbound window.
	maxOutbound int
	// ackStall fires if the inbound window stays empty and un-Acked; nil when healthy.
	// ackStallAfter is a field only so tests need not wait out the real threshold.
	ackStall      *time.Timer
	ackStallAfter time.Duration
}

// creditWindowCeilingFactor bounds the outbound window at a multiple of the purpose's initial
// credit. A plugin may run ahead of the host by a few windows (useful burst headroom) but not
// turn the grant path into an unbounded accumulator.
const creditWindowCeilingFactor = 4

// newFlowChannel is the only channel constructor. There is deliberately no bare variant: one
// used to exist for callers with no purpose to hand, and because it was the one production
// reached, every channel shipped with a nil credit window — no accounting inbound, no gating
// outbound, and an unbounded queue in the host. A channel and its flow control are one thing.
//
// writeFrame performs the actual kind=0x02 wire emission (Conn.WriteBinary bound to this
// channel's id); a fake here lets tests observe outbound writes without a real Conn. A nil clk
// uses the real wall clock.
func newFlowChannel(id uint32, purpose string, initialCredit int, throughputKbps int, clk clock, writeFrame func(payload []byte) error) *channel {
	c := &channel{
		id:     id,
		state:  channelStateOpen,
		notify: make(chan struct{}, 1),
		closed: make(chan struct{}),
	}
	c.purpose = purpose
	c.credit = newChannelCredit(initialCredit)
	c.throttle = newTokenBucket(throughputBytesPerSec(throughputKbps), clk)
	c.gate = newBackendGate(c.credit)
	c.writeFrame = writeFrame
	c.maxOutbound = initialCredit * creditWindowCeilingFactor
	c.ackStallAfter = ackStallTimeout
	return c
}

// Gate returns the capacity signal a purpose backend's read loop blocks on before pulling more
// data from its upstream source. Never nil.
func (c *channel) Gate() *backendGate { return c.gate }

// Send emits one outbound kind=0x02 frame, blocking until outbound credit is available and the
// throughput token bucket allows it. Frame size never affects credit cost — a 1-byte and a
// near-maxFrameSize frame both cost exactly one credit unit.
//
// Blocking is uniform across purposes: the backend's own upstream read loop is expected to
// already be gated on Gate(), so this is the host-enforced backstop, not the primary pressure
// point. Nothing is ever dropped here — a frame the caller handed over is a frame that gets
// sent, or an error (see channel_backpressure.go on why no purpose gets latest-frame-wins).
func (c *channel) Send(ctx context.Context, payload []byte) error {
	// Never emit before the channel.open reply is on the wire: see the `opened` field. Gates
	// only the host's outbound emission; the plugin's presumed initial credit (the convention
	// both sides start from) is unchanged, so the handshake it drives is not delayed. A nil gate
	// (the un-gated default) skips the wait entirely.
	if c.opened != nil {
		select {
		case <-c.opened:
		case <-c.closed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := c.credit.AcquireOutbound(ctx); err != nil {
		return err
	}
	return c.writeOut(ctx, payload)
}

// gateUntilOpened engages the outbound send gate: Send blocks until markOpened. Called by
// OpenDataPath before the backend is wired, so no Send can race the assignment.
func (c *channel) gateUntilOpened() {
	c.opened = make(chan struct{})
}

// markOpened releases the outbound send gate once the channel.open reply has been written to
// the wire. Idempotent, and a no-op for an un-gated channel.
func (c *channel) markOpened() {
	c.openOnce.Do(func() {
		if c.opened != nil {
			close(c.opened)
		}
	})
}

func (c *channel) writeOut(ctx context.Context, payload []byte) error {
	if c.throttle != nil {
		if delay := c.throttle.Reserve(len(payload)); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			case <-c.closed:
				return nil
			}
		}
	}
	if c.writeFrame == nil {
		return errNoChannelWriter
	}
	return c.writeFrame(payload)
}

// errNoChannelWriter reports a channel with no way to emit. Returning nil here — pretending
// the bytes went out — is how the bus silently swallowed every outbound frame.
var errNoChannelWriter = errors.New("ipc: channel has no frame writer")

// grantInbound widens the plugin's send window by n frames. The matching kind=0x03 emission is
// the caller's (channelDataPath.Ack): granting locally before telling the plugin keeps the host
// strictly more permissive than the plugin believes, which is the safe side of that race — the
// reverse order would let a frame arrive against credit the host has not yet recorded and get
// the plugin killed for a protocol violation it did not commit.
func (c *channel) grantInbound(n int) {
	c.credit.GrantInbound(n)
	c.disarmAckStall()
}

// ackStallTimeout is how long a fully-drained inbound window may sit un-Acked before the host
// says so. It is a diagnostic threshold, not a deadline: nothing is torn down.
const ackStallTimeout = 30 * time.Second

// armAckStall starts the watchdog for a window that has just run out. Ack is the backend's
// obligation, and a backend that forgets it stalls its own channel — silently, and only after
// the first window's worth of frames, which is about as misleading as a bug can be. The
// watchdog turns that into one line naming the channel and purpose.
//
// The timer only exists while a channel is actually starved, and Ack cancels it, so a healthy
// channel carries no timer at all.
func (c *channel) armAckStall() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ackStall != nil || c.state == channelStateClosed {
		return
	}
	c.ackStall = time.AfterFunc(c.ackStallAfter, func() {
		slog.Warn("plugin channel stalled: inbound credit exhausted with no Ack",
			"channelId", c.id, "purpose", c.purpose, "after", c.ackStallAfter,
			"detail", "the purpose backend must Ack each frame once it reaches its consumer; until it does, the plugin cannot send")
	})
}

func (c *channel) disarmAckStall() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ackStall != nil {
		c.ackStall.Stop()
		c.ackStall = nil
	}
}

// receiveCreditFrame decodes one inbound kind=0x03 frame and applies it. Its single concern is
// the ADR-011 wire format: 4B channelId + 4B credit. headerChannelID is the id the frame header
// carried (and which the mux already routed on); the payload repeats it, and the two disagreeing
// means the plugin is granting credit for a channel other than the one it addressed.
//
// Frame length is not re-checked here: validateFrameHeader (frame_reader.go) already refuses any
// kind=0x03 frame whose length is not exactly 8 before a payload is ever read.
func (c *channel) receiveCreditFrame(headerChannelID uint32, payload []byte) error {
	declaredChannelID := binary.BigEndian.Uint32(payload[0:4])
	credit := binary.BigEndian.Uint32(payload[4:8])

	if declaredChannelID != headerChannelID {
		return newProtocolViolation("credit frame for channel %d declares channel %d", headerChannelID, declaredChannelID)
	}
	if credit == 0 {
		return newProtocolViolation("credit frame for channel %d grants zero credit", headerChannelID)
	}
	return c.ReceiveCredit(credit)
}

// ReceiveCredit applies an inbound kind=0x03 replenishment to this channel's outbound
// window, unblocking any Send/Gate waiter. Its single concern is the credit window itself:
// the grant is refused if it would push the window past its ceiling, which is a protocol
// violation rather than a value to silently clamp — clamping would hide a plugin whose own
// bookkeeping has diverged from the host's.
func (c *channel) ReceiveCredit(n uint32) error {
	if err := c.credit.ReplenishOutbound(int(n), c.maxOutbound); err != nil {
		return newProtocolViolation("channel %d: %v (ceiling %d)", c.id, err, c.maxOutbound)
	}
	return nil
}

// errFrameTooLarge reports a frame above its purpose's ceiling. channelMux.Dispatch turns it into
// a protocol violation, which is what it is: deterministic, always the plugin's own bug, and
// fail-fast exactly as an oversized length already is at the header level (ADR-011 §2a). It is
// pointedly not ErrRateLimited -- that reads as backpressure and would send a plugin author
// hunting a throughput problem that does not exist.
func errFrameTooLarge(purpose string, got, max int) error {
	return fmt.Errorf("frame of %d bytes exceeds the %d byte ceiling for purpose %q", got, max, purpose)
}

// deliver enqueues an inbound frame. After Close(), further deliveries are dropped as
// no-ops per ADR-011 §Session lifecycle coupling ("the host ignores all further frames
// for that channelId"), not errors. For kind=0x02 frames on a channel with Stage 5 flow
// control configured, delivery is also accounted against the credit the host granted the
// plugin; sending past that grant is a protocol violation, enforced host-side regardless of
// what the plugin claims locally (defense in depth, ADR-011 §2b).
func (c *channel) deliver(kind byte, payload []byte) error {
	c.mu.Lock()
	if c.state == channelStateClosed {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if kind == domainplugin.FrameKindBinary {
		// The per-purpose size rule is enforced here, before the frame is accounted, queued or
		// seen by any backend, because this is the first place the purpose and the payload are
		// both known: validateFrameHeader has only the header's 1 MiB ceiling to go on, and a
		// backend that checked would be checking after the host already holds the bytes. It is
		// deliberately the only such check -- an embed-stream frame above the cap cannot reach the
		// embed sink, so the sink's own guard is unreachable defence in depth rather than a second
		// competing rule (D2).
		if max := domainplugin.MaxFrameBytesForPurpose(c.purpose); len(payload) > max {
			return errFrameTooLarge(c.purpose, len(payload), max)
		}
		if err := c.credit.ConsumeInbound(); err != nil {
			return err
		}
		if c.credit.AvailableInbound() == 0 {
			// The plugin has spent its whole window. From here it cannot send again until
			// someone Acks; if nobody does, that is worth saying out loud.
			c.armAckStall()
		}
	}

	c.mu.Lock()
	if c.state == channelStateClosed {
		c.mu.Unlock()
		return nil
	}
	buf := append([]byte(nil), payload...)
	c.queue = append(c.queue, channelFrame{Kind: kind, Payload: buf})
	c.mu.Unlock()

	select {
	case c.notify <- struct{}{}:
	default:
	}
	return nil
}

// Recv blocks until a frame is available or the channel is closed with an empty queue.
// The bool return is false only when the channel is closed and has nothing left queued.
func (c *channel) Recv() (channelFrame, bool) {
	for {
		c.mu.Lock()
		if len(c.queue) > 0 {
			f := c.queue[0]
			c.queue = c.queue[1:]
			c.mu.Unlock()
			return f, true
		}
		closed := c.state == channelStateClosed
		c.mu.Unlock()
		if closed {
			return channelFrame{}, false
		}
		select {
		case <-c.notify:
		case <-c.closed:
		}
	}
}

// Close transitions the channel to closed. Safe to call concurrently and repeatedly from
// both the local side and the remote-close path racing each other; only the first caller
// performs the state transition, every other call is a no-op.
func (c *channel) Close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.state = channelStateClosed
		// A closed channel is not a stalled one: nobody is waiting on credit any more.
		if c.ackStall != nil {
			c.ackStall.Stop()
			c.ackStall = nil
		}
		c.mu.Unlock()
		close(c.closed)
	})
}

// Closed reports whether Close() has been called.
func (c *channel) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == channelStateClosed
}
