package ipc

import (
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
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

	// Bucket starts full (burst allowance = 1 second worth): it goes out with no delay.
	if d := b.Reserve(bytesPerSec); d != 0 {
		t.Fatalf("initial burst of %d bytes wanted delay %v, want 0", bytesPerSec, d)
	}

	// No time has passed: the next byte must wait exactly as long as one byte takes to refill.
	got := b.Reserve(1)
	want := time.Second / bytesPerSec
	if got != want {
		t.Fatalf("delay for 1 byte past the burst = %v, want %v (1/%d of a second)", got, want, bytesPerSec)
	}

	// Half a second's refill buys exactly half a second's bytes. The single byte of debt above
	// is still owed, so the budget is one byte short of the full half-second.
	clk.Advance(time.Second / 2)
	if d := b.Reserve(bytesPerSec/2 - 1); d != 0 {
		t.Fatalf("half a second's refill wanted delay %v for half a second's bytes, want 0", d)
	}
	if d := b.Reserve(1); d == 0 {
		t.Fatal("expected the byte past the refilled budget to require a wait")
	}
}

// TestChannelThrottleContinuousWritesStayWithinRatePerWindow models the real sender: reserve,
// wait exactly as told, send. The bytes that make it out over a window must not exceed the
// configured rate — that is the property the throttle exists for, and it must hold when the
// caller waits on a computed delay rather than polling.
func TestChannelThrottleContinuousWritesStayWithinRatePerWindow(t *testing.T) {
	clk := newFakeClock()
	const bytesPerSec = 500
	const frame = 10
	b := newTokenBucket(bytesPerSec, clk)

	// Spend the initial burst so the window below measures steady state only.
	if d := b.Reserve(bytesPerSec); d != 0 {
		t.Fatalf("draining the burst wanted delay %v, want 0", d)
	}

	start := clk.Now()
	sent := 0
	for i := 0; i < 200; i++ {
		clk.Advance(b.Reserve(frame)) // the sender waits exactly as long as it was told
		sent += frame
	}

	elapsed := clk.Now().Sub(start).Seconds()
	if elapsed <= 0 {
		t.Fatal("expected the throttle to have forced the sender to wait at all")
	}
	if rate := float64(sent) / elapsed; rate > bytesPerSec*1.01 {
		t.Fatalf("steady-state rate %.1f bytes/sec exceeds configured %d bytes/sec", rate, bytesPerSec)
	}
}

// TestChannelThrottleReserveSerializesConcurrentSenders: reservations book their debt under one
// lock, so two senders queue into the rate instead of both being told "go now".
func TestChannelThrottleReserveSerializesConcurrentSenders(t *testing.T) {
	clk := newFakeClock()
	const bytesPerSec = 1000
	b := newTokenBucket(bytesPerSec, clk)

	if d := b.Reserve(bytesPerSec); d != 0 {
		t.Fatalf("burst wanted delay %v, want 0", d)
	}

	first := b.Reserve(500)
	second := b.Reserve(500)
	if first == 0 || second == 0 {
		t.Fatalf("both reservations past the burst must wait, got %v and %v", first, second)
	}
	if second <= first {
		t.Fatalf("second reservation delay %v must exceed the first's %v; equal delays mean both would fire at once and double the rate", second, first)
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
