package ipc

import (
	"context"
	"errors"
	"sync"
)

// ErrCreditExceeded is returned when a plugin sends more kind=0x02 frames than the host has
// granted it. Host-enforced server-side regardless of what the plugin claims locally
// (defense in depth, ADR-011 §2b) — never trust the sender's own bookkeeping.
var ErrCreditExceeded = errors.New("ipc: plugin sent more frames than granted credit")

// ErrCreditWindowOverflow is returned when a plugin's kind=0x03 grants would push the
// outbound window past its ceiling. Without it the window is an unbounded accumulator a
// plugin can drive to integer overflow with a grant loop.
var ErrCreditWindowOverflow = errors.New("ipc: credit grant exceeds the outbound window ceiling")

// channelCredit tracks two independent ADR-011 §2b credit windows for one channel:
//
//   - outbound: how many kind=0x02 frames the host may still send before it must wait for a
//     kind=0x03 replenishment from the plugin.
//   - inbound: how many kind=0x02 frames the host has granted the plugin permission to send,
//     decremented as they actually arrive.
//
// Credit is counted in frames, never bytes: an individual frame's size (from 1 byte up to
// maxFrameSize) never affects either count, which is what decouples flow control from
// maxFrameSize per ADR-011 §Limits.
type channelCredit struct {
	mu       sync.Mutex
	outbound int
	inbound  int
	// grown is closed and replaced whenever outbound credit increases, broadcasting to every
	// waiter at once. A sync.Cond would be the obvious fit but cannot be selected on alongside
	// ctx.Done(), which forced a helper goroutine per blocking call to translate cancellation
	// into a Broadcast — a goroutine per frame on the send path.
	grown chan struct{}
}

func newChannelCredit(initial int) *channelCredit {
	return &channelCredit{
		outbound: initial,
		inbound:  initial,
		grown:    make(chan struct{}),
	}
}

// signalGrownLocked wakes every waiter. Must be called with c.mu held.
func (c *channelCredit) signalGrownLocked() {
	close(c.grown)
	c.grown = make(chan struct{})
}

// awaitOutbound blocks until outbound credit grows or ctx is done. take is retried under the
// lock after each wake-up and reports whether the waiter is satisfied; it may consume state.
func (c *channelCredit) awaitOutbound(ctx context.Context, take func() bool) error {
	for {
		c.mu.Lock()
		if take() {
			c.mu.Unlock()
			return nil
		}
		grown := c.grown
		c.mu.Unlock()

		if ctx == nil {
			<-grown
			continue
		}
		select {
		case <-grown:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// AcquireOutbound blocks until at least one outbound credit unit is available, then consumes
// it. Returns ctx.Err() if ctx is cancelled first.
func (c *channelCredit) AcquireOutbound(ctx context.Context) error {
	return c.awaitOutbound(ctx, func() bool {
		if c.outbound <= 0 {
			return false
		}
		c.outbound--
		return true
	})
}

// TryAcquireOutbound consumes one outbound credit unit if available, without blocking.
func (c *channelCredit) TryAcquireOutbound() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.outbound <= 0 {
		return false
	}
	c.outbound--
	return true
}

// WaitOutboundAvailable blocks until outbound credit is non-zero without consuming it. This
// is the non-consuming peek a backend's upstream read loop gates on (channel_backpressure.go)
// — consumption happens separately, at actual send time.
func (c *channelCredit) WaitOutboundAvailable(ctx context.Context) error {
	return c.awaitOutbound(ctx, func() bool {
		return c.outbound > 0
	})
}

// AvailableInbound reports how many more frames the plugin is still allowed to send.
func (c *channelCredit) AvailableInbound() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inbound
}

// AvailableOutbound reports the current outbound credit without consuming it.
func (c *channelCredit) AvailableOutbound() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outbound
}

// ReplenishOutbound adds n credit units, e.g. from an inbound kind=0x03 frame, and wakes any
// caller blocked in AcquireOutbound/WaitOutboundAvailable. It is the only way to grow the
// window, and it always takes a ceiling: an unbounded variant would be a plugin-driven
// accumulator, and having both would leave the bounded one merely conventional.
//
// The grant is refused whole (ErrCreditWindowOverflow, window untouched) rather than clamped,
// and the check is against the post-add total rather than n alone, so a drip of small grants
// cannot walk the window past the ceiling either.
func (c *channelCredit) ReplenishOutbound(n, ceiling int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.outbound+n > ceiling {
		return ErrCreditWindowOverflow
	}
	c.outbound += n
	c.signalGrownLocked()
	return nil
}

// ConsumeInbound accounts for one arriving kind=0x02 frame against the credit the host has
// granted the plugin. Returns ErrCreditExceeded once the plugin has sent past that grant.
func (c *channelCredit) ConsumeInbound() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inbound <= 0 {
		return ErrCreditExceeded
	}
	c.inbound--
	return nil
}

// GrantInbound increases the credit granted to the plugin, e.g. as the host drains/processes
// received frames; the caller is responsible for emitting the matching kind=0x03 frame.
func (c *channelCredit) GrantInbound(n int) {
	c.mu.Lock()
	c.inbound += n
	c.mu.Unlock()
}
