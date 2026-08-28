package usecase

import (
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/ratelimit"
)

// The embed pump answers a rate-limit refusal by waiting a fixed embedDeliverRetryInterval and
// trying the same frame again (deliverFrame). A poll like that can only ever move one bucketful
// per interval, so the throughput it can reach is burst/interval — the configured rate never
// enters into it. Sizing the burst at one frame therefore capped the tunnel at
// 64 KiB / 25 ms ~ 2.5 MiB/s against a configured 32 MiB/s, and delivered it in 40 visible
// stop-go chunks a second, which is what a scrolling remote desktop tore on.
//
// This is the arithmetic that has to hold, and it spans two packages, so neither constant can be
// changed alone without this failing: raising the retry interval breaks it exactly as surely as
// shrinking the burst.
func TestTunnelBurstCoversAFullRetryInterval(t *testing.T) {
	const nanosPerSecond = int64(time.Second)
	rate := int64(domain.DefaultTunnelBandwidthBytesPerSec)
	required := rate * int64(embedDeliverRetryInterval) / nanosPerSecond

	if int64(domain.DefaultTunnelBurstBytes) < required {
		t.Fatalf("DefaultTunnelBurstBytes = %d, want >= %d (%d B/s x %v): a poll-based consumer "+
			"can move at most one burst per retry interval, so a smaller burst caps the tunnel at "+
			"%d B/s regardless of the configured rate",
			domain.DefaultTunnelBurstBytes, required,
			rate, embedDeliverRetryInterval,
			int64(domain.DefaultTunnelBurstBytes)*nanosPerSecond/int64(embedDeliverRetryInterval))
	}
}

// The same property through the limiter that actually runs in production rather than through its
// constants. A token bucket starts full and refilling only ever adds, so consuming exactly one
// burst is admitted no matter how long the loop takes — no wall-clock dependency.
func TestSessionLimiterAdmitsOneBurstOfFullSizeFrames(t *testing.T) {
	limiter := ratelimit.Factory{}.New(
		domain.DefaultTunnelBandwidthBytesPerSec,
		domain.DefaultTunnelBurstBytes,
	)

	const frame = domain.MaxTunnelFrameSize
	frames := domain.DefaultTunnelBurstBytes / frame
	for i := 0; i < frames; i++ {
		if !limiter.AllowN(frame) {
			t.Fatalf("frame %d of %d refused after %d B: a fresh session limiter must admit a "+
				"full burst, and a burst below one retry interval is what throttled the tunnel to "+
				"a fraction of its configured rate", i+1, frames, i*frame)
		}
	}
}

// A burst is a bandwidth allowance, not a buffer: nothing downstream of the limiter is sized by
// it. Frames are held by the embed-stream credit window and the WebSocket send queue, both
// counted in frames, and the queue is pinned to the window (embedWSSendQueueDepth). Widening the
// burst therefore cannot widen host memory — the guard that says so is this one, because the
// tempting way to "fix" a future memory finding is to shrink the burst again.
func TestWidenedBurstDoesNotWidenTheSendQueue(t *testing.T) {
	if got := embedWSSendQueueDepth * domain.MaxTunnelFrameSize; got > 1<<20 {
		t.Fatalf("worst-case queued bytes per tunnel = %d, want <= 1 MiB; the send queue is what "+
			"bounds host memory here, and it is counted in frames rather than in burst bytes", got)
	}
}
