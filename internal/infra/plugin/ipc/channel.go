package ipc

import (
	"context"
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

	// Stage 5 flow control. Nil (credit == nil) means "no flow control configured" — the
	// zero-arg newChannel used by Stage 2/3 callers and existing tests keeps working
	// unrestricted; only channels built via newFlowChannel gate outbound sends.
	purpose   string
	credit    *channelCredit
	throttle  *tokenBucket
	policy    exhaustionPolicy
	gate      *backendGate
	staging   *stagingBuffer
	writeFrame func(payload []byte) error
}

func newChannel(id uint32) *channel {
	return &channel{
		id:     id,
		state:  channelStateOpen,
		notify: make(chan struct{}, 1),
		closed: make(chan struct{}),
	}
}

// newFlowChannel builds a channel with Stage 5 credit/backpressure/throttle wiring applied.
// writeFrame performs the actual kind=0x02 wire emission (e.g. Conn.WriteBinary bound to
// this channel's id); a fake here lets tests observe outbound writes without a real Conn.
// A nil clk uses the real wall clock.
func newFlowChannel(id uint32, purpose string, initialCredit int, throughputKbps int, clk clock, writeFrame func(payload []byte) error) *channel {
	c := newChannel(id)
	c.purpose = purpose
	c.credit = newChannelCredit(initialCredit)
	c.throttle = newTokenBucket(throughputBytesPerSec(throughputKbps), clk)
	c.policy = policyForPurpose(purpose)
	c.gate = newBackendGate(c.credit)
	if c.policy == policyDropOldestUnsent {
		c.staging = newStagingBuffer(initialCredit)
	}
	c.writeFrame = writeFrame
	return c
}

// Gate returns the capacity signal a pause-upstream-read purpose backend's read loop blocks
// on (nil for channels without Stage 5 flow control configured).
func (c *channel) Gate() *backendGate { return c.gate }

// Staging returns the embed-stream drop-oldest staging buffer (nil for other purposes / for
// channels without Stage 5 flow control configured).
func (c *channel) Staging() *stagingBuffer { return c.staging }

// Send emits one outbound kind=0x02 frame, gated on the per-purpose ADR-011 §2b exhaustion
// policy and the throughput token bucket. Frame size never affects credit cost — a 1-byte
// and a near-maxFrameSize frame both cost exactly one credit unit.
//
// exec/tcp-relay/udp-relay (policyPauseUpstreamRead) block here until outbound credit is
// available: the backend's own upstream read loop is expected to already be gated on Gate(),
// so this is the host-enforced backstop, not the primary pressure point.
//
// embed-stream (policyDropOldestUnsent) never blocks: at credit 0 the newest frame is staged,
// evicting the oldest unsent one first (latest-frame-wins).
func (c *channel) Send(ctx context.Context, payload []byte) error {
	if c.credit == nil {
		return c.writeOut(payload)
	}
	if c.policy == policyDropOldestUnsent {
		if c.credit.TryAcquireOutbound() {
			return c.writeOut(payload)
		}
		c.staging.Push(payload)
		return nil
	}
	if err := c.credit.AcquireOutbound(ctx); err != nil {
		return err
	}
	return c.writeOut(payload)
}

func (c *channel) writeOut(payload []byte) error {
	if c.throttle != nil {
		for !c.throttle.Allow(len(payload)) {
			time.Sleep(time.Millisecond)
		}
	}
	if c.writeFrame == nil {
		return nil
	}
	return c.writeFrame(payload)
}

// ReceiveCredit applies an inbound kind=0x03 replenishment to this channel's outbound
// window, unblocking any Send/Gate waiter.
func (c *channel) ReceiveCredit(n uint32) {
	if c.credit == nil {
		return
	}
	c.credit.ReplenishOutbound(int(n))
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

	if kind == domainplugin.FrameKindBinary && c.credit != nil {
		if err := c.credit.ConsumeInbound(); err != nil {
			return err
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
