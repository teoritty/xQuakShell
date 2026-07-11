package ipc

import "sync"

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
}

func newChannel(id uint32) *channel {
	return &channel{
		id:     id,
		state:  channelStateOpen,
		notify: make(chan struct{}, 1),
		closed: make(chan struct{}),
	}
}

// deliver enqueues an inbound frame. After Close(), further deliveries are dropped as
// no-ops per ADR-011 §Session lifecycle coupling ("the host ignores all further frames
// for that channelId"), not errors.
func (c *channel) deliver(kind byte, payload []byte) {
	c.mu.Lock()
	if c.state == channelStateClosed {
		c.mu.Unlock()
		return
	}
	buf := append([]byte(nil), payload...)
	c.queue = append(c.queue, channelFrame{Kind: kind, Payload: buf})
	c.mu.Unlock()

	select {
	case c.notify <- struct{}{}:
	default:
	}
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
