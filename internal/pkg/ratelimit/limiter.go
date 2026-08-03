// Package ratelimit provides a token-bucket byte throughput limiter backed by golang.org/x/time/rate.
package ratelimit

import (
	"time"

	"golang.org/x/time/rate"

	"xquakshell/internal/domain"
)

var (
	_ domain.RateLimiter        = (*limiter)(nil)
	_ domain.RateLimiterFactory = Factory{}
)

type limiter struct {
	l *rate.Limiter
}

// New creates a byte throughput limiter with the given sustained rate and burst.
func New(bytesPerSec int, burst int) domain.RateLimiter {
	if bytesPerSec < 1 {
		bytesPerSec = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &limiter{
		l: rate.NewLimiter(rate.Limit(bytesPerSec), burst),
	}
}

// AllowN reports whether n bytes may pass through immediately.
func (l *limiter) AllowN(n int) bool {
	return l.l.AllowN(time.Now(), n)
}

// Factory implements domain.RateLimiterFactory for composition-root wiring.
type Factory struct{}

// New mints a per-session limiter.
func (Factory) New(bytesPerSec int, burst int) domain.RateLimiter {
	return New(bytesPerSec, burst)
}
