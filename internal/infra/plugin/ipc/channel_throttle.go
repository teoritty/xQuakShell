package ipc

import (
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// clock abstracts time so tokenBucket is testable without wall-clock sleeps.
type clock interface {
	Now() time.Time
}

// realClock is the production clock seam.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// throughputBytesPerSec resolves the ADR-011 §Limits host default (0 -> host default) into a
// bytes/sec rate. The "Kbps" field name (channel.maxThroughputKbps, DefaultChannelThroughputKbps)
// is a carry-over from the existing maxTunnelBandwidthKbps convention, but its value is
// actually KiB/s — DefaultChannelThroughputKbps == 32*1024 is documented as "32 MiB/s" per
// ADR-011 §Limits — so the conversion to bytes/sec is *1024, not a bits-to-bytes /8.
func throughputBytesPerSec(maxThroughputKbps int) int {
	kbps := maxThroughputKbps
	if kbps <= 0 {
		kbps = domainplugin.DefaultChannelThroughputKbps
	}
	return kbps * 1024
}

// tokenBucket enforces a byte/sec throughput cap on a channel's write path, independent of
// and in addition to the frame-count credit window (ADR-011 §2b): credit bounds how much can
// be in flight unacknowledged, this bounds sustained throughput over time.
type tokenBucket struct {
	mu       sync.Mutex
	rate     float64 // bytes/sec
	capacity float64 // burst ceiling: one second's worth of tokens
	tokens   float64
	last     time.Time
	clk      clock
}

// newTokenBucket builds a bucket capped at bytesPerSec, starting full. A nil clk uses the
// real wall clock; tests inject a fake to assert exact byte/sec math deterministically.
func newTokenBucket(bytesPerSec int, clk clock) *tokenBucket {
	if clk == nil {
		clk = realClock{}
	}
	rate := float64(bytesPerSec)
	return &tokenBucket{
		rate:     rate,
		capacity: rate,
		tokens:   rate,
		last:     clk.Now(),
		clk:      clk,
	}
}

func (b *tokenBucket) refill() {
	now := b.clk.Now()
	elapsed := now.Sub(b.last).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now
}

// Allow reports whether n bytes may be sent right now, consuming tokens if so. It never
// blocks: the caller (channel.go) decides how to wait between failed attempts, keeping this
// file responsible for the rate math only (SRP).
func (b *tokenBucket) Allow(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	cost := float64(n)
	if b.tokens < cost {
		return false
	}
	b.tokens -= cost
	return true
}
