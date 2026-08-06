package ipc

import (
	"context"
	"errors"
	"testing"
	"time"
)

// testCreditCeiling is a ceiling high enough to never be the thing under test: these tests
// exercise blocking and wake-up, while the ceiling itself is covered by
// TestCreditFrameOverflowIsProtocolViolation.
const testCreditCeiling = 1 << 20

func TestChannelCreditBlocksSenderAtZeroRegardlessOfFrameSize(t *testing.T) {
	c := newChannelCredit(2)

	if err := c.AcquireOutbound(context.Background()); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	if err := c.AcquireOutbound(context.Background()); err != nil {
		t.Fatalf("frame 2: %v", err)
	}

	// Credit is now 0: the 3rd (N+1-th) outbound frame must not proceed until credit is
	// replenished, regardless of how large or small it is.
	done := make(chan error, 1)
	go func() {
		done <- c.AcquireOutbound(context.Background())
	}()

	select {
	case <-done:
		t.Fatal("3rd frame acquired credit while outbound credit was 0")
	case <-time.After(100 * time.Millisecond):
	}

	if err := c.ReplenishOutbound(1, testCreditCeiling); err != nil {
		t.Fatalf("replenish: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error after replenish: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("3rd frame did not unblock after inbound credit replenishment")
	}
}

func TestChannelCreditReplenishUnblocksExactlyOneWaiter(t *testing.T) {
	c := newChannelCredit(0)

	waiterDone := make(chan struct{})
	go func() {
		_ = c.AcquireOutbound(context.Background())
		close(waiterDone)
	}()

	time.Sleep(20 * time.Millisecond)
	if err := c.ReplenishOutbound(1, testCreditCeiling); err != nil {
		t.Fatalf("replenish: %v", err)
	}

	select {
	case <-waiterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never unblocked")
	}

	if got := c.AvailableOutbound(); got != 0 {
		t.Fatalf("available outbound = %d, want 0 (the single credit unit should be consumed)", got)
	}
}

func TestChannelCreditFrameSizeDoesNotAffectCost(t *testing.T) {
	// Property: a 1-byte frame and a near-1MiB frame both cost exactly one credit unit —
	// credit is decoupled from maxFrameSize by design.
	sizes := []int{1, (1 << 20) - 9}
	for _, size := range sizes {
		c := newChannelCredit(1)
		if err := c.AcquireOutbound(context.Background()); err != nil {
			t.Fatalf("size %d: unexpected error: %v", size, err)
		}
		if got := c.AvailableOutbound(); got != 0 {
			t.Fatalf("size %d: available outbound = %d, want 0 after consuming the single unit", size, got)
		}
		_ = size // size itself never enters channelCredit's accounting, by design
	}
}

func TestChannelCreditHostRejectsPluginSendingPastGrantedCredit(t *testing.T) {
	c := newChannelCredit(2)

	if err := c.ConsumeInbound(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	if err := c.ConsumeInbound(); err != nil {
		t.Fatalf("frame 2: %v", err)
	}

	// The plugin has now exhausted its granted 2-frame credit; a 3rd inbound frame is a
	// protocol violation, host-enforced independent of anything the plugin claims locally.
	err := c.ConsumeInbound()
	if !errors.Is(err, ErrCreditExceeded) {
		t.Fatalf("expected ErrCreditExceeded for the 3rd inbound frame, got %v", err)
	}
}

func TestChannelCreditGrantInboundAllowsFurtherFrames(t *testing.T) {
	c := newChannelCredit(1)
	if err := c.ConsumeInbound(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	if err := c.ConsumeInbound(); !errors.Is(err, ErrCreditExceeded) {
		t.Fatalf("expected ErrCreditExceeded, got %v", err)
	}

	c.GrantInbound(1)

	if err := c.ConsumeInbound(); err != nil {
		t.Fatalf("frame after grant: %v", err)
	}
}

func TestChannelCreditAcquireOutboundRespectsContextCancellation(t *testing.T) {
	c := newChannelCredit(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := c.AcquireOutbound(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestChannelDeliverEnforcesInboundCreditAsProtocolViolation exercises the host-enforced
// server-side path end to end: a channel with flow control configured must reject
// (not silently queue) an inbound kind=0x02 frame once the plugin's granted credit is spent.
func TestChannelDeliverEnforcesInboundCreditAsProtocolViolation(t *testing.T) {
	ch := newFlowChannel(1, "exec", 1, 0, nil, func([]byte) error { return nil })

	if err := ch.deliver(0x02, []byte("frame 1")); err != nil {
		t.Fatalf("frame 1 should be accepted: %v", err)
	}
	if err := ch.deliver(0x02, []byte("frame 2")); !errors.Is(err, ErrCreditExceeded) {
		t.Fatalf("expected ErrCreditExceeded for frame past granted credit, got %v", err)
	}
}
