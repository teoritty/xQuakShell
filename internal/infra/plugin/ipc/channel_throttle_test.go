package ipc

import (
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// fakeClock is an injectable, manually-advanced clock so throughput tests are deterministic
// and never depend on wall-clock sleeps.
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(0, 0)} }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestChannelThrottleDoesNotExceedConfiguredBytesPerSecond(t *testing.T) {
	clk := newFakeClock()
	const bytesPerSec = 1000
	b := newTokenBucket(bytesPerSec, clk)

	// Bucket starts full (burst allowance = 1 second worth); drain it immediately.
	if !b.Allow(bytesPerSec) {
		t.Fatal("expected the initial burst allowance to permit bytesPerSec bytes immediately")
	}

	// No time has passed: any further byte must be refused (would exceed X bytes/sec).
	if b.Allow(1) {
		t.Fatal("expected throttle to refuse further bytes with no elapsed time")
	}

	// Advance the fake clock by exactly one second: exactly bytesPerSec more bytes should be
	// allowed, no more.
	clk.Advance(time.Second)
	if !b.Allow(bytesPerSec) {
		t.Fatal("expected a full second's worth of bytes to be allowed after 1s elapsed")
	}
	if b.Allow(1) {
		t.Fatal("expected throttle to refuse bytes beyond the 1s budget")
	}
}

func TestChannelThrottleContinuousWritesStayWithinRatePerWindow(t *testing.T) {
	clk := newFakeClock()
	const bytesPerSec = 500
	b := newTokenBucket(bytesPerSec, clk)

	// Drain the initial burst first so the window below measures steady-state rate only.
	for b.Allow(50) {
	}

	const windowSeconds = 5
	const tick = 10 * time.Millisecond
	ticksPerSecond := int(time.Second / tick)
	totalSent := 0

	for s := 0; s < windowSeconds; s++ {
		sentThisSecond := 0
		for i := 0; i < ticksPerSecond; i++ {
			clk.Advance(tick)
			for b.Allow(10) {
				sentThisSecond += 10
			}
		}
		if sentThisSecond > bytesPerSec {
			t.Fatalf("second %d: sent %d bytes, exceeds configured rate %d bytes/sec", s, sentThisSecond, bytesPerSec)
		}
		totalSent += sentThisSecond
	}

	if totalSent > bytesPerSec*windowSeconds {
		t.Fatalf("total sent %d over %ds window exceeds %d bytes/sec budget", totalSent, windowSeconds, bytesPerSec*windowSeconds)
	}
}

func TestChannelThrottleZeroKbpsUsesHostDefault(t *testing.T) {
	got := throughputBytesPerSec(0)
	want := domainplugin.DefaultChannelThroughputKbps * 1024
	if got != want {
		t.Fatalf("throughputBytesPerSec(0) = %d, want host default %d", got, want)
	}
}

func TestChannelThrottleExplicitKbpsIsRespected(t *testing.T) {
	got := throughputBytesPerSec(64)
	if got != 64*1024 {
		t.Fatalf("throughputBytesPerSec(64) = %d, want %d", got, 64*1024)
	}
}

func TestChannelThrottleGatesChannelSendWithFakeClock(t *testing.T) {
	clk := newFakeClock()
	var writtenBytes int
	writeDone := make(chan struct{})

	ch := newFlowChannel(1, domainplugin.PurposeExec, 100, 1, clk, func(p []byte) error {
		writtenBytes += len(p)
		close(writeDone)
		return nil
	})
	// 1 Kbps(=KiB/s) host convention => 1024 bytes/sec budget, bucket starts full.
	payload := make([]byte, 1024)

	done := make(chan error, 1)
	go func() {
		done <- ch.Send(t.Context(), payload)
	}()

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the first send (within the initial full bucket) to go out promptly")
	}
	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writtenBytes != len(payload) {
		t.Fatalf("written bytes = %d, want %d", writtenBytes, len(payload))
	}
}
