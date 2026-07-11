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
	cond     *sync.Cond
	outbound int
	inbound  int
}

func newChannelCredit(initial int) *channelCredit {
	c := &channelCredit{outbound: initial, inbound: initial}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// AcquireOutbound blocks until at least one outbound credit unit is available, then consumes
// it. Returns ctx.Err() if ctx is cancelled first.
func (c *channelCredit) AcquireOutbound(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.waitLocked(ctx, func() bool {
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
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.waitLocked(ctx, func() bool {
		return c.outbound > 0
	})
}

// waitLocked loops cond.Wait() until attempt returns true (consuming state as needed under
// the same lock) or ctx is done. Must be called with c.mu held.
func (c *channelCredit) waitLocked(ctx context.Context, attempt func() bool) error {
	if attempt() {
		return nil
	}
	if ctx == nil {
		for !attempt() {
			c.cond.Wait()
		}
		return nil
	}

	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.cond.Broadcast()
			c.mu.Unlock()
		case <-done:
		}
		close(stopped)
	}()
	defer func() {
		close(done)
		<-stopped
	}()

	for !attempt() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		c.cond.Wait()
	}
	return nil
}

// AvailableOutbound reports the current outbound credit without consuming it.
func (c *channelCredit) AvailableOutbound() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outbound
}

// ReplenishOutbound adds n credit units, e.g. from an inbound kind=0x03 frame, and wakes any
// caller blocked in AcquireOutbound/WaitOutboundAvailable.
func (c *channelCredit) ReplenishOutbound(n int) {
	c.mu.Lock()
	c.outbound += n
	c.cond.Broadcast()
	c.mu.Unlock()
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
